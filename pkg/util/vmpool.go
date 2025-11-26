package util

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"

	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	networkingv1 "k8s.io/api/networking/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	kubevirtv1 "kubevirt.io/api/core/v1"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func GenerateVMPool(iObj *seederv1alpha1.InventoryTemplate) ([]*kubevirtv1.VirtualMachine, error) {
	poolName := iObj.Name

	var vmObjs []*kubevirtv1.VirtualMachine
	// generate network interfaces

	// GenerateDataVolumeTemplates, Disk and Volume Info in one go
	// so this can be added to the VirtualMachineTemplateSpec

	for vmCount := 0; vmCount < int(iObj.Spec.VMSpec.Count); vmCount++ {
		dataVolumeTemplates := []kubevirtv1.DataVolumeTemplateSpec{}
		disks := []kubevirtv1.Disk{}
		volumes := []kubevirtv1.Volume{}
		for i, diskReq := range iObj.Spec.VMSpec.Disks {
			// generate datavolumetemplates
			// us vm count as dataVolumeTemplate name is used to eventually
			// create associated PVC as a result needs to include vmCount
			// to uniquely identify it for the VM
			diskName := fmt.Sprintf("%s-disk-%d-%d", poolName, vmCount, i)
			dataVolumeTemplate := &kubevirtv1.DataVolumeTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: diskName,
				},
				Spec: cdiv1.DataVolumeSpec{
					Source: &cdiv1.DataVolumeSource{
						Blank: &cdiv1.DataVolumeBlankImage{},
					},
					PVC: &corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteMany,
						},
						StorageClassName: &diskReq.StorageClass,
						Resources: corev1.VolumeResourceRequirements{
							Requests: map[corev1.ResourceName]resource.Quantity{
								corev1.ResourceStorage: diskReq.Size,
							},
						},
					},
				},
			}
			dataVolumeTemplates = append(dataVolumeTemplates, *dataVolumeTemplate)
			// generate disk spec
			// disk boot order is set to after networks
			// so is len of networks count + current count + 1
			// as current count starts at 0
			bootOrder := len(iObj.Spec.VMSpec.Networks) + i + 1
			disk := kubevirtv1.Disk{
				Name: diskName,
				DiskDevice: kubevirtv1.DiskDevice{
					Disk: &kubevirtv1.DiskTarget{
						Bus: diskReq.Bus,
					},
				},
				BootOrder: ptr.To(uint(bootOrder)),
			}
			disks = append(disks, disk)
			volume := kubevirtv1.Volume{
				Name: diskName,
				VolumeSource: kubevirtv1.VolumeSource{
					DataVolume: &kubevirtv1.DataVolumeSource{
						Name: diskName,
					},
				},
			}
			volumes = append(volumes, volume)
		}

		// generate network definitions
		// since nic is specific to VM no need to include vmCount here
		interfaces := []kubevirtv1.Interface{}
		networks := []kubevirtv1.Network{}
		for i, nic := range iObj.Spec.VMSpec.Networks {
			nicName := fmt.Sprintf("%s-nic-%d", poolName, i)

			// define network attachment for VM
			network := kubevirtv1.Network{
				Name: nicName,
				NetworkSource: kubevirtv1.NetworkSource{
					Multus: &kubevirtv1.MultusNetwork{
						NetworkName: nic.VMNetwork,
					},
				},
			}
			networks = append(networks, network)

			hwAddr, err := GenerateMacAddress()
			if err != nil {
				return nil, fmt.Errorf("error generating mac address for vm pool interfaces: %w", err)
			}
			// define nic's and boot order
			bootOrder := i + 1
			nicInterface := kubevirtv1.Interface{
				Name:       nicName,
				Model:      nic.NICModel,
				BootOrder:  ptr.To(uint(bootOrder)),
				MacAddress: hwAddr.String(),
				InterfaceBindingMethod: kubevirtv1.InterfaceBindingMethod{
					Bridge: &kubevirtv1.InterfaceBridge{},
				},
			}
			interfaces = append(interfaces, nicInterface)
		}

		vm := &kubevirtv1.VirtualMachine{
			TypeMeta: metav1.TypeMeta{
				Kind:       kubevirtv1.VirtualMachineGroupVersionKind.Kind,
				APIVersion: kubevirtv1.VirtualMachineGroupVersionKind.GroupVersion().String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%d", poolName, vmCount),
				Namespace: iObj.Spec.VMSpec.Namespace,
				Labels: map[string]string{
					seederv1alpha1.InventoryUUIDLabelKey: string(iObj.GetUID()),
				},
			},
			Spec: kubevirtv1.VirtualMachineSpec{
				RunStrategy:         ptr.To(kubevirtv1.RunStrategyHalted),
				DataVolumeTemplates: dataVolumeTemplates,
				Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							seederv1alpha1.InventoryUUIDLabelKey: string(iObj.GetUID()),
						},
					},
					Spec: kubevirtv1.VirtualMachineInstanceSpec{

						Domain: kubevirtv1.DomainSpec{
							Features: &kubevirtv1.Features{
								ACPI: kubevirtv1.FeatureState{
									Enabled: ptr.To(true),
								},
							},
							Devices: kubevirtv1.Devices{
								Disks:      disks,
								Interfaces: interfaces,
							},
							Firmware: &kubevirtv1.Firmware{
								Bootloader: &kubevirtv1.Bootloader{
									EFI: &kubevirtv1.EFI{
										SecureBoot: ptr.To(false),
									},
								},
							},
							CPU: &kubevirtv1.CPU{
								Sockets:    1,
								Cores:      iObj.Spec.VMSpec.CPU,
								Threads:    1,
								MaxSockets: 1,
							},
							Memory: &kubevirtv1.Memory{
								Guest: &iObj.Spec.VMSpec.Memory,
							},
						},
						Volumes:  volumes,
						Networks: networks,
					},
				},
			},
		}

		vmObjs = append(vmObjs, vm)
	}

	return vmObjs, nil
}

// GeneratePoolName will generate a unique pool which is combination of inventorytemplate name and namespace
func GeneratePoolName(i *seederv1alpha1.InventoryTemplate) string {
	return i.Name
}

// GenerateMacAddress will generate a mac address while adding the interface
// this will be used subsequently to generate inventory definition
// which can then be pxe booted via seeder
// copied from https://github.com/harvester/harvester/blob/master/pkg/util/network/common.go#L68
func GenerateMacAddress() (net.HardwareAddr, error) {
	buf := make([]byte, 6)

	_, err := rand.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("error reading random bytes: %w", err)
	}

	// Set the Local Bit (the 2nd least significant bit) to 1.
	// Binary: 00000010 (Hex: 0x02).
	buf[0] |= 0x02

	// Clear the Multicast Bit (the least significant bit) to 0, ensuring Unicast.
	// Binary: 11111110 (Hex: 0xFE).
	buf[0] &= 0xFE

	return net.HardwareAddr(buf), nil
}

// generateKubeconfigFromSecret is used to generate a remote client from the kubeconfig provided in secret reference
// of an InventoryTemplate
func GenerateRemoteKubeconfigFromSecret(ctx context.Context, reference *corev1.SecretReference, localClient client.Client) (client.Client, error) {
	secret := &corev1.Secret{}
	if err := localClient.Get(ctx, types.NamespacedName{Name: reference.Name, Namespace: reference.Namespace}, secret); err != nil {
		return nil, fmt.Errorf("error fetching secret for secret reference %v: %w", reference, err)
	}

	kcBytes, ok := secret.Data[seederv1alpha1.SecretKubeconfigFieldKey]
	if !ok {
		return nil, fmt.Errorf("no key %s found in secret %s/%s", seederv1alpha1.SecretKubeconfigFieldKey, secret.Namespace, secret.Name)
	}

	clientOverride, err := clientcmd.NewClientConfigFromBytes(kcBytes)
	if err != nil {
		return nil, fmt.Errorf("error generating client override: %w", err)
	}

	restConfig, err := clientOverride.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("error generating rest config: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := kubevirtv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("error registering kubevirt v1 to client scheme: %w", err)
	}

	if err := nadv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("error registered net-attach-def v1 to client scheme: %w", err)
	}

	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("error registering client-go scheme: %w", err)
	}

	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("error registering apiextensions/v1 to client scheme: %w", err)
	}
	return client.New(restConfig, client.Options{Scheme: scheme})
}

// GenerateIngressPool will generate ingress objects for each VM exposed by kubevirt bmc
// based on ingress template available in kubevirtbmc docs
func GenerateIngressAndInventoryPool(vmObjs []*kubevirtv1.VirtualMachine, endpoint string, inventoryTemplate *seederv1alpha1.InventoryTemplate) ([]*networkingv1.Ingress, []*seederv1alpha1.Inventory) {
	var ingressObjects []*networkingv1.Ingress
	var inventoryObjects []*seederv1alpha1.Inventory
	for _, vm := range vmObjs {
		hostName := fmt.Sprintf("%s-ingress.%s.sslip.io", vm.Name, endpoint)
		ingress := &networkingv1.Ingress{
			TypeMeta: metav1.TypeMeta{
				APIVersion: networkingv1.SchemeGroupVersion.String(),
				Kind:       "Ingress",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      vm.Name,
				Namespace: seederv1alpha1.KubeBMCNS,
				Annotations: map[string]string{
					seederv1alpha1.KubeBMCIngressAnnotationKey: seederv1alpha1.KubeBMCIngressAnnotationValue,
				},
				Labels: map[string]string{
					seederv1alpha1.InventoryUUIDLabelKey: string(inventoryTemplate.GetUID()),
				},
			},
			Spec: networkingv1.IngressSpec{
				IngressClassName: &inventoryTemplate.Spec.VMSpec.IngressClassName,
				TLS: []networkingv1.IngressTLS{
					{
						Hosts: []string{
							hostName,
						},
						SecretName: fmt.Sprintf("%s-%s-virtbmc-tls", vm.Namespace, vm.Name),
					},
				},
				Rules: []networkingv1.IngressRule{
					{
						Host: hostName,
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{
									{
										Path:     "/",
										PathType: ptr.To(networkingv1.PathTypePrefix),
										Backend: networkingv1.IngressBackend{
											Service: &networkingv1.IngressServiceBackend{
												Name: fmt.Sprintf("%s-%s-virtbmc", vm.Namespace, vm.Name),
												Port: networkingv1.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		ingressObjects = append(ingressObjects, ingress)

		// default disk will be /dev/sda unless virtio driver is used in which case it changes to /dev/vda
		// scsi and sata based devices show up as /dev/sda in the guest
		var disk = "/dev/sda"
		if inventoryTemplate.Spec.VMSpec.Disks[0].Bus == kubevirtv1.DiskBusVirtio {
			disk = "/dev/vda"
		}

		inventory := &seederv1alpha1.Inventory{
			TypeMeta: metav1.TypeMeta{
				Kind:       seederv1alpha1.InventoryGroupVersionKind.Kind,
				APIVersion: seederv1alpha1.InventoryGroupVersionKind.GroupVersion().String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      vm.Name,
				Namespace: inventoryTemplate.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: seederv1alpha1.InventoryTemplateGroupVersionKind.GroupVersion().String(),
						Kind:       seederv1alpha1.InventoryTemplateGroupVersionKind.Kind,
						Name:       inventoryTemplate.Name,
						UID:        inventoryTemplate.UID,
					},
				},
				Labels: map[string]string{
					seederv1alpha1.InventoryUUIDLabelKey: string(inventoryTemplate.GetUID()),
				},
			},
			Spec: seederv1alpha1.InventorySpec{
				PrimaryDisk:                   disk,
				ManagementInterfaceMacAddress: vm.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress,
				BaseboardManagementSpec: rufio.MachineSpec{
					Connection: rufio.Connection{
						Host: hostName,
						AuthSecretRef: corev1.SecretReference{
							Name:      inventoryTemplate.Name,
							Namespace: inventoryTemplate.Namespace,
						},
					},
				},
			},
		}

		inventoryObjects = append(inventoryObjects, inventory)
	}

	return ingressObjects, inventoryObjects
}

// GetIngressEndpoint attempts to get the harvester ingress-expose vip which can then be used
// for subsequently creating ingress endpoints. these ingress endpoints are subsequently used to generate the
// related inventory object
func GetIngressEndpoint(ctx context.Context, harvesterClient client.Client) (string, error) {
	var endpoint string
	svc := &corev1.Service{}
	err := harvesterClient.Get(ctx, types.NamespacedName{Name: seederv1alpha1.IngressExposeService, Namespace: seederv1alpha1.KubeSystemNS}, svc)
	if err != nil {
		return endpoint, fmt.Errorf("error fetching svc %s: %w", seederv1alpha1.IngressExposeService, err)
	}

	if len(svc.Status.LoadBalancer.Ingress) == 0 {
		return endpoint, fmt.Errorf("no ingress ip found, erroring to requeue until an ingress-expose ip is available")
	}

	endpoint = svc.Status.LoadBalancer.Ingress[0].IP
	return endpoint, nil
}

func GenerateTemplateSecret(inventoryTemplate *seederv1alpha1.InventoryTemplate) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      inventoryTemplate.Name,
			Namespace: inventoryTemplate.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: seederv1alpha1.InventoryTemplateGroupVersionKind.GroupVersion().String(),
					Kind:       seederv1alpha1.InventoryTemplateGroupVersionKind.Kind,
					Name:       inventoryTemplate.Name,
					UID:        inventoryTemplate.UID,
				},
			},
		},
		StringData: map[string]string{
			"username": "admin",
			"password": "password",
		},
	}
}

// GenerateInventoryTemplates will generate inventory template objects based on the
// inventory template config available in nested cluster spec
func GenerateInventoryTemplates(n *seederv1alpha1.NestedCluster) []*seederv1alpha1.InventoryTemplate {
	var inventoryTemplates []*seederv1alpha1.InventoryTemplate
	for _, itConfig := range n.Spec.InventoryTemplateConfig {
		inventoryTemplate := &seederv1alpha1.InventoryTemplate{
			TypeMeta: metav1.TypeMeta{
				Kind:       seederv1alpha1.InventoryTemplateGroupVersionKind.Kind,
				APIVersion: seederv1alpha1.InventoryTemplateGroupVersionKind.GroupVersion().String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", n.Name, itConfig.Name),
				Namespace: n.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: seederv1alpha1.NestedClusterGroupVersionKind.GroupVersion().String(),
						Kind:       seederv1alpha1.NestedClusterGroupVersionKind.Kind,
						Name:       n.Name,
						UID:        n.UID,
					},
				},
				Labels: map[string]string{
					seederv1alpha1.NestedClusterUIDLabelKey: string(n.UID),
				},
			},
			Spec: itConfig.InventoryTemplateSpec,
		}
		inventoryTemplates = append(inventoryTemplates, inventoryTemplate)
	}
	return inventoryTemplates
}

func GenerateClusterFromNestedCluster(n *seederv1alpha1.NestedCluster) *seederv1alpha1.Cluster {

	cluster := &seederv1alpha1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       seederv1alpha1.ClusterGroupVersionKind.Kind,
			APIVersion: seederv1alpha1.ClusterGroupVersionKind.GroupVersion().String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      n.Name,
			Namespace: n.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: seederv1alpha1.NestedClusterGroupVersionKind.GroupVersion().String(),
					Kind:       seederv1alpha1.NestedClusterGroupVersionKind.Kind,
					Name:       n.Name,
					UID:        n.UID,
				},
			},
		},
		Spec: seederv1alpha1.ClusterSpec{
			HarvesterVersion: n.Spec.HarvesterVersion,
			ImageURL:         n.Spec.ImageURL,
			VIPConfig:        n.Spec.VIPConfig,
			ClusterConfig:    n.Spec.ClusterConfig,
		},
	}

	// generate node configs
	var nodeConfigs []seederv1alpha1.NodeConfig

	for _, tc := range n.Spec.InventoryTemplateConfig {
		poolName := fmt.Sprintf("%s-%s", n.Name, tc.Name)
		for i := 0; i < int(tc.InventoryTemplateSpec.VMSpec.Count); i++ {
			nodeConfig := seederv1alpha1.NodeConfig{
				InventoryReference: seederv1alpha1.ObjectReference{
					Name:      fmt.Sprintf("%s-%d", poolName, i),
					Namespace: n.Namespace,
				},
				AddressPoolReference: tc.AddressPoolReference,
			}
			nodeConfigs = append(nodeConfigs, nodeConfig)
		}
	}
	cluster.Spec.Nodes = nodeConfigs
	return cluster
}
