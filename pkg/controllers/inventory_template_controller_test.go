package controllers

import (
	"fmt"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Create and validate inventory template", func() {
	var i *seederv1alpha1.InventoryTemplate

	memory, err := resource.ParseQuantity("8Gi")
	Expect(err).NotTo(HaveOccurred())
	disk, err := resource.ParseQuantity("300Gi")
	Expect(err).NotTo(HaveOccurred())

	BeforeEach(func() {
		i = &seederv1alpha1.InventoryTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "template-test",
				Namespace: "default",
			},
			Spec: seederv1alpha1.InventoryTemplateSpec{
				Credentials: &corev1.SecretReference{
					Name:      localHarvesterSecretName,
					Namespace: "default",
				},
				VMSpec: seederv1alpha1.VMSpec{
					CPU:    4,
					Memory: memory,
					Disks: []seederv1alpha1.DiskConfig{
						{
							Bus:          "virtio",
							Size:         disk,
							StorageClass: storageClass,
						},
					},
					Networks: []seederv1alpha1.NetworkConfig{
						{
							NICModel:  "virtio",
							VMNetwork: nadName,
						},
					},
					Namespace:        "default",
					Count:            4,
					IngressClassName: "nginx",
				},
			},
		}

		Eventually(func() error {
			return k8sClient.Create(ctx, i)
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	It("validate inventory template objects exist", func() {
		By("checking vm's exist in default namespace of test environment", func() {
			Eventually(func() error {
				vmList := &kubevirtv1.VirtualMachineList{}
				err := k8sClient.List(ctx, vmList, client.InNamespace(i.Spec.VMSpec.Namespace), client.MatchingLabels{seederv1alpha1.InventoryUUIDLabelKey: string(i.GetUID())})
				if err != nil {
					return err
				}
				if len(vmList.Items) != int(i.Spec.VMSpec.Count) {
					return fmt.Errorf("expected to find %d vm's but found %d", int(i.Spec.VMSpec.Count), len(vmList.Items))
				}
				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		// ingress are always generated in seederv1alpha1.KubeBMCNS namespace
		// as that is where the service pointing to VM are defined
		By("checking ingress objects in default namespace of test environment", func() {
			Eventually(func() error {
				ingressList := &networkingv1.IngressList{}
				err := k8sClient.List(ctx, ingressList, client.InNamespace(seederv1alpha1.KubeBMCNS), client.MatchingLabels{seederv1alpha1.InventoryUUIDLabelKey: string(i.GetUID())})
				if err != nil {
					return err
				}
				if len(ingressList.Items) != int(i.Spec.VMSpec.Count) {
					return fmt.Errorf("expected to find %d ingresses but found %d", int(i.Spec.VMSpec.Count), len(ingressList.Items))
				}
				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})
		By("checking inventory objects in default namespace of test environment", func() {
			Eventually(func() error {
				inventoryList := &seederv1alpha1.InventoryList{}
				err := k8sClient.List(ctx, inventoryList, client.InNamespace(i.Namespace), client.MatchingLabels{seederv1alpha1.InventoryUUIDLabelKey: string(i.GetUID())})
				if err != nil {
					return err
				}
				if len(inventoryList.Items) != int(i.Spec.VMSpec.Count) {
					return fmt.Errorf("expected to find %d inventories but found %d", int(i.Spec.VMSpec.Count), len(inventoryList.Items))
				}
				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		// validate GC operations
		By("deleting the cluster object", func() {
			Eventually(func() error {
				return k8sClient.Delete(ctx, i)
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		// ensure VM's are removed from target namespace
		By("checking vm's are removed from default namespace of test environment", func() {
			Eventually(func() error {
				vmList := &kubevirtv1.VirtualMachineList{}
				err := k8sClient.List(ctx, vmList, client.InNamespace(i.Spec.VMSpec.Namespace), client.MatchingLabels{seederv1alpha1.InventoryUUIDLabelKey: string(i.GetUID())})
				if err != nil {
					return err
				}
				if len(vmList.Items) != 0 {
					return fmt.Errorf("expected to find %d vm's but found %d", 0, len(vmList.Items))
				}
				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		// ingress are always generated in seederv1alpha1.KubeBMCNS namespace
		// as that is where the service pointing to VM are defined
		By("checking ingress objects in default namespace of test environment", func() {
			Eventually(func() error {
				ingressList := &networkingv1.IngressList{}
				err := k8sClient.List(ctx, ingressList, client.InNamespace(seederv1alpha1.KubeBMCNS), client.MatchingLabels{seederv1alpha1.InventoryUUIDLabelKey: string(i.GetUID())})
				if err != nil {
					return err
				}
				if len(ingressList.Items) != 0 {
					return fmt.Errorf("expected to find %d ingresses but found %d", int(i.Spec.VMSpec.Count), len(ingressList.Items))
				}
				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})
	})
})
