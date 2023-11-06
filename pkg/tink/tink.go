package tink

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/pkg/errors"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
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

	var m string
	if strings.Contains(c.Spec.HarvesterVersion, "v1.0") {
		m, err = generateMetaDataV10(c.Spec.ConfigURL, c.Spec.HarvesterVersion, i.Spec.ManagementInterfaceMacAddress, mode,
			i.Spec.PrimaryDisk, c.Status.ClusterAddress, c.Status.ClusterToken, i.Status.GeneratedPassword, c.Spec.ImageURL, c.Spec.ClusterConfig.Nameservers, c.Spec.ClusterConfig.SSHKeys)
	} else {
		m, err = generateMetaDataV11(c.Spec.ConfigURL, c.Spec.HarvesterVersion, i.Spec.ManagementInterfaceMacAddress, mode,
			i.Spec.PrimaryDisk, c.Status.ClusterAddress, c.Status.ClusterToken, i.Status.GeneratedPassword, c.Spec.ImageURL, c.Spec.ClusterConfig.Nameservers, c.Spec.ClusterConfig.SSHKeys)
	}
	if err != nil {
		return nil, errors.Wrap(err, "error during metadata generation")
	}

	hw = &tinkv1alpha1.Hardware{
		ObjectMeta: metav1.ObjectMeta{
			Name:      i.Name,
			Namespace: i.Namespace,
		},
		Spec: tinkv1alpha1.HardwareSpec{
			Interfaces: []tinkv1alpha1.Interface{
				{
					Netboot: &tinkv1alpha1.Netboot{
						AllowPXE: &[]bool{true}[0],
						OSIE: &tinkv1alpha1.OSIE{
							BaseURL: c.Spec.ImageURL,
						},
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
					Userdata: m,
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

// generateMetaDataV10 is a wrapper to generate metadata for nodes to create or join a cluster
func generateMetaDataV10(configURL, version, hwAddress, mode, disk, vip, token, password, imageurl string, Nameservers, SSHKeys []string) (metadata string, err error) {

	var tmpStruct struct {
		ConfigURL   string
		HWAddress   string
		Mode        string
		Disk        string
		VIP         string
		Token       string
		SSHKeys     []string
		Nameservers []string
		Password    string
		IsoURL      string
	}
	var output bytes.Buffer
	tmpStruct.ConfigURL = configURL
	tmpStruct.HWAddress = hwAddress
	tmpStruct.Mode = mode
	tmpStruct.Disk = disk
	tmpStruct.VIP = vip
	tmpStruct.Token = token
	tmpStruct.Password = password
	tmpStruct.SSHKeys = SSHKeys
	tmpStruct.Nameservers = Nameservers
	tmpStruct.Password = password
	endpoint := defaultISOURL
	if imageurl != "" {
		endpoint = imageurl
	}
	tmpStruct.IsoURL = fmt.Sprintf("%s/%s/harvester-%s-amd64.iso", endpoint, version, version)

	var metaDataStruct = `{{ if ne .ConfigURL ""}}harvester.install.config_url={{ .ConfigURL }}{{end}} harvester.install.networks.harvester-mgmt.interfaces="hwAddr:{{ .HWAddress }}" ip=dhcp harvester.install.networks.harvester-mgmt.method=dhcp harvester.install.networks.harvester-mgmt.bond_options.mode=balance-tlb harvester.install.networks.harvester-mgmt.bond_options.miimon=100 console=ttyS1,115200  harvester.install.mode={{ .Mode }} harvester.token={{ .Token }} harvester.os.password={{ .Password }} {{ range $v := .SSHKeys}}harvester.os.ssh_authorized_keys=\"- {{ $v }} \ "{{ end }}{{range $v := .Nameservers}}harvester.os.dns_nameservers={{ $v }} {{end}} harvester.install.vip={{ .VIP }} harvester.install.vip_mode=static harvester.install.iso_url={{ .IsoURL }} harvester.install.device={{ .Disk }} {{if eq .Mode "join"}}harvester.server_url={{ printf "https://%s:8443" .VIP }}{{end}}`

	metadataTmpl := template.Must(template.New("MetaData").Parse(metaDataStruct))

	err = metadataTmpl.Execute(&output, tmpStruct)

	if err != nil {
		return metadata, err
	}

	metadata = output.String()
	return metadata, nil
}

func generateMetaDataV11(configURL, version, hwAddress, mode, disk, vip, token, password, imageurl string, Nameservers, SSHKeys []string) (metadata string, err error) {

	var tmpStruct struct {
		ConfigURL   string
		HWAddress   string
		Mode        string
		Disk        string
		VIP         string
		Token       string
		SSHKeys     []string
		Nameservers []string
		Password    string
		IsoURL      string
	}
	var output bytes.Buffer
	tmpStruct.ConfigURL = configURL
	tmpStruct.HWAddress = hwAddress
	tmpStruct.Mode = mode
	tmpStruct.Disk = disk
	tmpStruct.VIP = vip
	tmpStruct.Token = token
	tmpStruct.Password = password
	tmpStruct.SSHKeys = SSHKeys
	tmpStruct.Nameservers = Nameservers
	tmpStruct.Password = password
	endpoint := defaultISOURL
	if imageurl != "" {
		endpoint = imageurl
	}
	tmpStruct.IsoURL = fmt.Sprintf("%s/%s/harvester-%s-amd64.iso", endpoint, version, version)

	var metaDataStruct = `{{ if ne .ConfigURL ""}}harvester.install.config_url={{ .ConfigURL }}{{end}} harvester.install.management_interface.interfaces="hwAddr:{{ .HWAddress }}" ip=dhcp harvester.install.management_interface.method=dhcp harvester.management_interface.bond_options.mode=balance-tlb harvester.install.management_interface.bond_options.miimon=100 console=ttyS1,115200  harvester.install.mode={{ .Mode }} harvester.token={{ .Token }} harvester.os.password={{ .Password }} {{ range $v := .SSHKeys}}harvester.os.ssh_authorized_keys=\"- {{ $v }} \ "{{ end }}{{range $v := .Nameservers}}harvester.os.dns_nameservers={{ $v }} {{end}} harvester.install.vip={{ .VIP }} harvester.install.vip_mode=static harvester.install.iso_url={{ .IsoURL }} harvester.install.device={{ .Disk }} {{if eq .Mode "join"}}harvester.server_url={{ printf "https://%s:443" .VIP }}{{end}} harvester.scheme_version=1`

	metadataTmpl := template.Must(template.New("MetaData").Parse(metaDataStruct))

	err = metadataTmpl.Execute(&output, tmpStruct)

	if err != nil {
		return metadata, err
	}

	metadata = output.String()
	return metadata, nil
}
