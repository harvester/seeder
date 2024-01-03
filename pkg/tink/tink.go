package tink

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/harvester/harvester-installer/pkg/config"
	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/util"
)

const (
	defaultLeaseTime    = 86400
	defaultArch         = "x86_64"
	defaultFacilityCode = "on_prem"
	defaultDistro       = "harvester"
	defaultISOURL       = "https://releases.rancher.com/harvester/"
	v11Prefix           = "v1.1"
	defaultEvent        = "SUCCEEDED"
	defaultMethod       = "PUT"
)

// GenerateHWRequest will generate the tinkerbell Hardware type object
func GenerateHWRequest(i *seederv1alpha1.Inventory, c *seederv1alpha1.Cluster, seederDeploymentService *corev1.Service, tinkStackService *corev1.Service) (hw *tinkv1alpha1.Hardware, err error) {

	// generate metadata
	mode := "join"
	if util.ConditionExists(i, seederv1alpha1.HarvesterCreateNode) {
		mode = "create"
	}

	if len(seederDeploymentService.Status.LoadBalancer.Ingress) == 0 {
		return nil, fmt.Errorf("waiting for ingress to be populated on svc: %s", seederDeploymentService.Name)
	}
	bondOptions := make(map[string]string)
	if c.Spec.BondOptions == nil {
		bondOptions["mode"] = "balance-tlb"
		bondOptions["miimon"] = "100"
	}
	m, err := generateCloudConfig(c.Spec.ConfigURL, i.Spec.ManagementInterfaceMacAddress, mode, c.Status.ClusterAddress,
		c.Status.ClusterToken, i.Status.GeneratedPassword, i.Status.Address, i.Status.Netmask, i.Status.Gateway, c.Spec.ClusterConfig.Nameservers, c.Spec.ClusterConfig.SSHKeys, bondOptions, c.Spec.ImageURL, c.Spec.HarvesterVersion, seederDeploymentService.Status.LoadBalancer.Ingress[0].IP, i.Name, i.Namespace)

	if err != nil {
		return nil, fmt.Errorf("error during HW generation: %v", err)
	}

	hw = &tinkv1alpha1.Hardware{
		ObjectMeta: metav1.ObjectMeta{
			Name:      i.Name,
			Namespace: i.Namespace,
		},
		Spec: tinkv1alpha1.HardwareSpec{
			UserData: &m,
			Interfaces: []tinkv1alpha1.Interface{
				{
					Netboot: &tinkv1alpha1.Netboot{
						AllowPXE:      &[]bool{true}[0],
						AllowWorkflow: &[]bool{true}[0],
					},
					DHCP: &tinkv1alpha1.DHCP{
						MAC:       i.Spec.ManagementInterfaceMacAddress,
						Hostname:  fmt.Sprintf("%s-%s", i.Name, i.Namespace),
						LeaseTime: defaultLeaseTime,
						Arch:      defaultArch,
						UEFI:      true,
						IP: &tinkv1alpha1.IP{
							Address: i.Status.Address,
							Netmask: i.Status.Netmask,
							Gateway: i.Status.Gateway,
						},
					},
				},
			},
			Disks: []tinkv1alpha1.Disk{
				{
					Device: i.Spec.PrimaryDisk,
				},
			},
			Metadata: &tinkv1alpha1.HardwareMetadata{
				Facility: &tinkv1alpha1.MetadataFacility{
					FacilityCode: defaultFacilityCode,
				},
				Instance: &tinkv1alpha1.MetadataInstance{
					OperatingSystem: &tinkv1alpha1.MetadataInstanceOperatingSystem{
						Version: c.Spec.HarvesterVersion,
						Distro:  defaultDistro,
					},
				},
			},
		},
	}

	// if version is pre v1.2.x then hardware will use custom ipxe boot script and not workflow based provisioning
	if strings.HasPrefix(c.Spec.HarvesterVersion, v11Prefix) {
		customIPXEScript, err := generateIPXEScript(c.Spec.HarvesterVersion, c.Spec.ImageURL, fmt.Sprintf("http://%s:%s/2009-04-04/user-data",
			tinkStackService.Status.LoadBalancer.Ingress[0].IP, HegelDefaultPort), i.Spec.ManagementInterfaceMacAddress, i.Spec.PrimaryDisk, i.Status.Address, i.Status.Netmask, i.Status.Gateway)
		if err != nil {
			return nil, fmt.Errorf("error generating custom ipxe script for inventory %s: %v", i.Name, err)
		}
		for i := range hw.Spec.Interfaces {
			hw.Spec.Interfaces[i].Netboot.IPXE = &tinkv1alpha1.IPXE{
				Contents: customIPXEScript,
			}
		}
	}
	return hw, nil
}

// GenerateWorkflow binds the template associated with inventory to the workflow
// this needs to be done before the ipxe boot is performed to ensure correct workflow is executed on reboot
func GenerateWorkflow(i *seederv1alpha1.Inventory, c *seederv1alpha1.Cluster) (workflow *tinkv1alpha1.Workflow) {
	workflow = &tinkv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      i.Name,
			Namespace: i.Namespace,
		},
		Spec: tinkv1alpha1.WorkflowSpec{
			TemplateRef: i.Name,
			HardwareRef: i.Name,
			HardwareMap: map[string]string{
				"device_1": i.Spec.ManagementInterfaceMacAddress,
			},
		},
	}

	if c.Spec.ClusterConfig.CustomProvisioningTemplate != "" {
		workflow.Spec.TemplateRef = c.Spec.ClusterConfig.CustomProvisioningTemplate
	}

	return workflow
}

func generateCloudConfig(configURL, hwAddress, mode, vip, token, password, ip, subnetMask, gateway string, Nameservers, SSHKeys []string, bondOptions map[string]string, imageURL string, harvesterVersion string, webhookURL string, hwName string, hwNamespace string) (string, error) {
	hc := config.NewHarvesterConfig()
	hc.SchemeVersion = 1
	hc.Token = token
	if mode == "join" {
		hc.ServerURL = fmt.Sprintf("https://%s:443/", vip)
	} else {
		hc.Install.Vip = vip
		hc.Install.VipMode = "static"
	}
	hc.Install.Mode = mode
	hc.Install.ManagementInterface = config.Network{
		Method:       "static",
		IP:           ip,
		SubnetMask:   subnetMask,
		Gateway:      gateway,
		DefaultRoute: true,
		Interfaces: []config.NetworkInterface{
			{
				HwAddr: hwAddress,
			},
		},
	}
	hc.Install.ConfigURL = configURL
	hc.Install.Automatic = true
	hc.OS.Password = password
	hc.OS.DNSNameservers = Nameservers
	hc.OS.SSHAuthorizedKeys = SSHKeys
	hc.Install.ManagementInterface.BondOptions = bondOptions
	// for versions older than v1.2.x where streaming image mode is not available
	// we need to provide ISO URL
	if strings.HasPrefix(harvesterVersion, v11Prefix) {
		hc.Install.ConfigURL = "" // reset the config url
		hc.Install.ISOURL = fmt.Sprintf("%s/%s/harvester-%s-amd64.iso", imageURL, harvesterVersion, harvesterVersion)
		hc.Install.Webhooks = []config.Webhook{
			{
				Event:  defaultEvent,
				Method: defaultMethod,
				URL:    fmt.Sprintf("http://%s:%d/disable/%s/%s", webhookURL, seederv1alpha1.DefaultEndpointPort, hwNamespace, hwName),
			},
		}
	}
	hcBytes, err := yaml.Marshal(hc)
	if err != nil {
		return "", fmt.Errorf("error marshalling yaml: %v", err)
	}
	return string(hcBytes), nil
}

// generateIPXEScript will generate an inline ipxe script similar to https://github.com/harvester/ipxe-examples/blob/main/general/ipxe-create
// and uses the same for create / join of node
func generateIPXEScript(harvesterVersion, isoURL, hegelEndpoint, macAddress, disk, address, netmask, gateway string) (string, error) {

	ipxeTemplateStruct := struct {
		Version       string
		ISOURL        string
		HegelEndpoint string
		MacAddress    string
		Address       string
		Netmask       string
		Gateway       string
		Disk          string
	}{
		Version:       harvesterVersion,
		ISOURL:        isoURL,
		HegelEndpoint: hegelEndpoint,
		MacAddress:    macAddress,
		Disk:          disk,
		Address:       address,
		Netmask:       netmask,
		Gateway:       gateway,
	}

	var output bytes.Buffer

	ipxeTemplate := `#!ipxe
	set version {{ .Version }}
	set base {{ .ISOURL}}/{{ .Version }}
	dhcp
	iflinkwait -t 5000
	goto ${ifname}
	:net0
	set address ${net0/mac}
	goto setupboot
	:net1
	set address ${net1/mac}
	goto setupboot
	:net2
	set address ${net2/mac}
	goto setupboot
	:net3
	set address ${net3/mac}
	goto setupboot

	:setupboot
	kernel ${base}/harvester-${version}-vmlinuz-amd64 initrd=harvester-${version}-initrd-amd64 ip=dhcp net.ifnames=1 rd.cos.disable rd.noverifyssl BOOTIF={{ .MacAddress }} root=live:${base}/harvester-${version}-rootfs-amd64.squashfs harvester.install.management_interface.interfaces=hwAddr:{{ .MacAddress }} harvester.install.management_interface.method=static harvester.install.management_interface.ip={{ .Address }} harvester.install.management_interface.subnet_mask={{ .Netmask }} harvester.install.management_interface.gateway={{ .Gateway }} harvester.install.device={{ .Disk }} harvester.install.management_interface.bond_options.mode=balance-tlb harvester.install.management_interface.bond_options.miimon=100 console=tty1 harvester.install.automatic=true boot_cmd='echo include_ping_test=yes >> /etc/conf.d/net-online' harvester.install.config_url={{ .HegelEndpoint }}
	initrd ${base}/harvester-${version}-initrd-amd64
	boot
	`
	ipxeTmpl := template.Must(template.New("IPXE").Parse(ipxeTemplate))
	err := ipxeTmpl.Execute(&output, ipxeTemplateStruct)
	if err != nil {
		return "", fmt.Errorf("error generating ipxe template: %v", err)
	}
	return output.String(), nil
}
