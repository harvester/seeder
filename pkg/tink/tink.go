package tink

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"text/template"

	"github.com/harvester/harvester-installer/pkg/config"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/util"
)

const (
	defaultLeaseTime    = 86400
	defaultFacilityCode = "on_prem"
	defaultDistro       = "harvester"
	defaultISOURL       = "https://releases.rancher.com/harvester/"
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
	userdata, err := generateCloudConfig(c.Spec.ConfigURL, i.Spec.ManagementInterfaceMacAddress, mode, c.Status.ClusterAddress,
		c.Status.ClusterToken, i.Status.GeneratedPassword, i.Status.Address, i.Status.Netmask, i.Status.Gateway, c.Spec.Nameservers, c.Spec.SSHKeys, bondOptions, c.Spec.ImageURL, c.Spec.HarvesterVersion, seederDeploymentService.Status.LoadBalancer.Ingress[0].IP, i.Name, i.Namespace, c.Spec.StreamImageMode, c.Spec.WipeDisks, c.Spec.VlanID, i.Spec.Arch, i.Spec.PrimaryDisk, fmt.Sprintf("%s-%s", i.Name, i.Namespace))

	if err != nil {
		return nil, fmt.Errorf("error during HW generation: %v", err)
	}

	// work around needed since boots represents amd64 arch as x86_64
	var hwArch string
	if i.Spec.Arch == "amd64" {
		hwArch = "x86_64"
	} else {
		hwArch = "aarch64"
	}

	hw = &tinkv1alpha1.Hardware{
		ObjectMeta: metav1.ObjectMeta{
			Name:      i.Name,
			Namespace: i.Namespace,
		},
		Spec: tinkv1alpha1.HardwareSpec{
			UserData: &userdata,
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
						Arch:      hwArch,
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

	// if not using StreamImage mode then define a custom ipxe url with info needed to provision harvester
	if !c.Spec.StreamImageMode {
		customIPXEScript, err := generateIPXEScript(c.Spec.HarvesterVersion, c.Spec.ImageURL, fmt.Sprintf("http://%s:%s/2009-04-04/user-data",
			tinkStackService.Status.LoadBalancer.Ingress[0].IP, HegelDefaultPort), i.Spec.ManagementInterfaceMacAddress, i.Spec.Arch, c.Spec.VlanID)
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

	if c.Spec.CustomProvisioningTemplate != "" {
		workflow.Spec.TemplateRef = c.Spec.CustomProvisioningTemplate
	}

	return workflow
}

func generateCloudConfig(configURL, hwAddress, mode, vip, token, password, ip, subnetMask, gateway string, Nameservers, SSHKeys []string, bondOptions map[string]string, imageURL string, harvesterVersion string, webhookURL string, hwName string, hwNamespace string, streamImage bool, wipeDisks bool, vlanID int, arch string, disk string, hostname string) (string, error) {
	hc := config.NewHarvesterConfig()
	if configURL != "" {
		if err := readConfigURL(hc, configURL); err != nil {
			return "", err
		}
	}
	hc.SchemeVersion = 1
	hc.Token = token
	if mode == "join" {
		hc.ServerURL = fmt.Sprintf("https://%s:443", vip)
	} else {
		hc.Vip = vip
		hc.VipMode = "static"
	}
	hc.Mode = mode
	hc.ManagementInterface = config.Network{
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
	if vlanID > 0 {
		hc.ManagementInterface.VlanID = vlanID
	}
	hc.Automatic = true
	hc.Password = password
	hc.DNSNameservers = append(hc.DNSNameservers, Nameservers...)
	hc.SSHAuthorizedKeys = append(hc.SSHAuthorizedKeys, SSHKeys...)
	hc.Hostname = hostname
	if vlanID > 1 {
		hc.AfterInstallChrootCommands = []string{fmt.Sprintf("grub2-editenv /oem/grubenv set extra_cmdline=\"ifname=netboot:%s\"", hwAddress)}
	}
	hc.ManagementInterface.BondOptions = bondOptions
	hc.WipeAllDisks = hc.WipeAllDisks || wipeDisks
	// append installation disk to WipeDisksList if wipeDisks is called at cluster level or via config url
	// this should address https://github.com/harvester/harvester/issues/9536
	if hc.WipeAllDisks {
		hc.WipeDisksList = append(hc.WipeDisksList, disk)
	}
	hc.Device = disk
	hc.SkipChecks = true
	// for versions older than v1.2.x where streaming image mode is not available
	// we need to provide ISO URL
	if !streamImage {
		//hc.Install.ConfigURL = "" // reset the config url
		hc.ISOURL = fmt.Sprintf("%s/%s/harvester-%s-%s.iso", imageURL, harvesterVersion, harvesterVersion, arch)
		hc.Webhooks = []config.Webhook{
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
func generateIPXEScript(harvesterVersion, isoURL, hegelEndpoint, macAddress, arch string, vlanID int) (string, error) {

	ipxeTemplateStruct := struct {
		Version       string
		ISOURL        string
		HegelEndpoint string
		MacAddress    string
		Arch          string
		VlanID        int
	}{
		Version:       harvesterVersion,
		ISOURL:        isoURL,
		HegelEndpoint: hegelEndpoint,
		MacAddress:    macAddress,
		Arch:          arch,
		VlanID:        vlanID,
	}

	var output bytes.Buffer

	ipxeTemplate := `#!ipxe
set version {{ .Version }}
set base {{ .ISOURL}}/{{ .Version }}
set arch {{ .Arch }}
dhcp
iflinkwait -t 5000
kernel ${base}/harvester-${version}-vmlinuz-${arch} initrd=harvester-${version}-initrd-${arch} ip=dhcp net.ifnames=1 rd.cos.disable rd.noverifyssl BOOTIF={{ .MacAddress }} root=live:${base}/harvester-${version}-rootfs-${arch}.squashfs console=tty1 harvester.install.automatic=true boot_cmd='echo include_ping_test=yes >> /etc/conf.d/net-online' harvester.install.config_url={{ .HegelEndpoint }} {{if gt .VlanID 1}}ifname=netboot:{{ .MacAddress }}  vlan=vlan{{ .VlanID }}:netboot {{end}}
initrd ${base}/harvester-${version}-initrd-${arch}
boot
`
	ipxeTmpl := template.Must(template.New("IPXE").Parse(ipxeTemplate))
	err := ipxeTmpl.Execute(&output, ipxeTemplateStruct)
	if err != nil {
		return "", fmt.Errorf("error generating ipxe template: %v", err)
	}
	return output.String(), nil
}

func readConfigURL(hc *config.HarvesterConfig, url string) error {
	// FileTransport is needed to make it easier to run unit tests
	t := &http.Transport{}
	t.RegisterProtocol("file", http.NewFileTransport(http.Dir(".")))
	c := &http.Client{Transport: t}
	resp, err := c.Get(url)
	if err != nil {
		return fmt.Errorf("error fetching config url %s: %v", url, err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error during http call, status code: %v", resp.Status)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading url response body: %v", err)
	}
	err = yaml.Unmarshal(content, hc)
	return err
}
