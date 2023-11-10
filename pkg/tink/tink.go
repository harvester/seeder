package tink

import (
	"fmt"

	"github.com/harvester/harvester-installer/pkg/config"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	yaml "gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/util"
)

const (
	defaultLeaseTime    = 86400
	defaultArch         = "x86_64"
	defaultFacilityCode = "on_prem"
	defaultDistro       = "harvester"
	defaultISOURL       = "https://releases.rancher.com/harvester/"
)

// GenerateHWRequest will generate the tinkerbell Hardware type object
func GenerateHWRequest(i *seederv1alpha1.Inventory, c *seederv1alpha1.Cluster) (hw *tinkv1alpha1.Hardware, err error) {

	// generate metadata
	mode := "join"
	if util.ConditionExists(i, seederv1alpha1.HarvesterCreateNode) {
		mode = "create"
	}

	m, err := generateCloudConfig(c.Spec.ConfigURL, i.Spec.ManagementInterfaceMacAddress, mode, c.Status.ClusterAddress,
		c.Status.ClusterToken, i.Status.GeneratedPassword, i.Status.Address, i.Status.Netmask, i.Status.Gateway, c.Spec.ClusterConfig.Nameservers, c.Spec.ClusterConfig.SSHKeys)
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

func generateCloudConfig(configURL, hwAddress, mode, vip, token, password, ip, subnetMask, gateway string, Nameservers, SSHKeys []string) (string, error) {
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

	hcBytes, err := yaml.Marshal(hc)
	if err != nil {
		return "", fmt.Errorf("error marshalling yaml: %v", err)
	}
	return string(hcBytes), nil
}
