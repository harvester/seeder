package tink

import (
	"bytes"
	"fmt"
	bmaasv1alpha1 "github.com/harvester/bmaas/pkg/api/v1alpha1"
	"github.com/harvester/bmaas/pkg/util"
	"github.com/pkg/errors"
	tinkv1alpha1 "github.com/tinkerbell/tink/pkg/apis/core/v1alpha1"
	"html/template"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultLeaseTime    = 86400
	defaultArch         = "x86_64"
	defaultFacilityCode = "on_prem"
)

//GenerateHWRequest will generate the tinkerbell Hardware type object
func GenerateHWRequest(i *bmaasv1alpha1.Inventory, c *bmaasv1alpha1.Cluster) (hw *tinkv1alpha1.Hardware, err error) {

	// generate metadata
	mode := "join"
	if util.ConditionExists(i.Status.Conditions, bmaasv1alpha1.HarvesterCreateNode) {
		mode = "create"
	}

	m, err := generateMetaData(c.Spec.ConfigURL, c.Spec.HarvesterVersion, i.Spec.ManagementInterfaceMacAddress, mode,
		i.Spec.PrimaryDisk, c.Status.ClusterAddress, c.Status.ClusterToken, i.Status.GeneratedPassword, c.Spec.ClusterConfig.Nameservers, c.Spec.ClusterConfig.SSHKeys)
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
						AllowPXE:      &[]bool{true}[0],
						AllowWorkflow: &[]bool{true}[0],
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
					ID:       i.Status.HardwareID,
					Userdata: m,
					OperatingSystem: &tinkv1alpha1.MetadataInstanceOperatingSystem{
						Slug: c.Spec.HarvesterVersion,
					},
				},
			},
		},
	}

	return hw, nil
}

// generateMetaData is a wrapper to generate metadata for nodes to create or join a cluster
func generateMetaData(configURL, slug, hwAddress, mode, disk, vip, token, password string, Nameservers, SSHKeys []string) (metadata string, err error) {

	var tmpStruct struct {
		ConfigURL   string
		Slug        string
		HWAddress   string
		Mode        string
		Disk        string
		VIP         string
		Token       string
		SSHKeys     []string
		Nameservers []string
		Password    string
	}
	var output bytes.Buffer
	tmpStruct.ConfigURL = configURL
	tmpStruct.Slug = slug
	tmpStruct.HWAddress = hwAddress
	tmpStruct.Mode = mode
	tmpStruct.Disk = disk
	tmpStruct.VIP = vip
	tmpStruct.Token = token
	tmpStruct.Password = password
	tmpStruct.SSHKeys = SSHKeys
	tmpStruct.Nameservers = Nameservers
	tmpStruct.Password = password

	var metaDataStruct = `{"facility":{"facility_code":"onprem"},"instance":{"userdata":"harvester.install.config_url={{ .ConfigURL }} harvester.install.networks.harvester-mgmt.interfaces=\"hwAddr:{{ .HWAddress }}\" ip=dhcp net.ifnames=1 rd.cos.disable rd.noverifyssl harvester.install.networks.harvester-mgmt.method=dhcp harvester.install.networks.harvester-mgmt.bond_options.mode=balance-tlb harvester.install.networks.harvester-mgmt.bond_options.miimon=100 console=ttyS1,115200 harvester.install.automatic=true boot_cmd="echo include_ping_test=yes >> /etc/conf.d/net-online" harvester.install.mode={{ .Mode }} harvester.token={{ .Token }} harvester.os.password={{ .Password }} {{ range $v := .SSHKeys}}harvester.os.ssh_authorized_keys=\"- {{ $v }} \ "{{ end }}{{range $v := .Nameservers}}harvester.os.dnsNameservers={{ $v }} {{end}} harvester.install.vip={{ .VIP }} harvester.install.vipMode=static" ,"operating_system":{"slug":"{{ .Slug }}"}}}`

	metadataTmpl := template.Must(template.New("MetaData").Parse(metaDataStruct))

	err = metadataTmpl.Execute(&output, tmpStruct)

	if err != nil {
		return metadata, err
	}

	metadata = output.String()
	return metadata, nil
}
