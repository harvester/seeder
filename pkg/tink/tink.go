package tink

import (
	"bytes"
	"context"
	"fmt"
	bmaasv1alpha1 "github.com/harvester/bmaas/pkg/api/v1alpha1"
	"github.com/harvester/bmaas/pkg/util"
	"github.com/pkg/errors"
	hw "github.com/tinkerbell/tink/client"
	"github.com/tinkerbell/tink/protos/hardware"
	"html/template"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"net/url"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewClient(ctx context.Context, apiClient client.Client) (fullClient *hw.FullClient, err error) {
	var certURL, grpcAuth string
	cm := &v1.ConfigMap{}
	err = apiClient.Get(ctx, types.NamespacedName{Name: bmaasv1alpha1.TinkConfig, Namespace: bmaasv1alpha1.DefaultNS}, cm)
	if err != nil {
		return nil, err
	}

	certURL, ok := cm.Data["CERT_URL"]
	if !ok {
		return nil, fmt.Errorf("cert_url not found in configmap tinkConfig")
	}

	grpcAuth, ok = cm.Data["GRPC_AUTH_URL"]
	if !ok {
		return nil, fmt.Errorf("grpc_auth_url not found in configmap tinkConfig")
	}

	return NewClientFromEndpoints(certURL, grpcAuth)
}

func NewClientFromEndpoints(certURL, grpcAuth string) (fullClient *hw.FullClient, err error) {
	connOpts := &hw.ConnOptions{CertURL: certURL, GRPCAuthority: grpcAuth}

	grpcConn, err := hw.NewClientConn(connOpts)
	if err != nil {
		return nil, fmt.Errorf("error creating grpc clients: %v", err)
	}

	fullClient = hw.NewFullClient(grpcConn)

	return fullClient, err
}

func GenerateHWRequest(i *bmaasv1alpha1.Inventory, c *bmaasv1alpha1.Cluster) (hw *hardware.Hardware, err error) {

	networkInterfaces := &hardware.Hardware_Network_Interface{
		Netboot: &hardware.Hardware_Netboot{
			AllowPxe: true,
		},
	}

	// Specify non default location to load ISO's. If not present tinkerbell will load the same from releases.rancher.com
	if len(c.Spec.ImageURL) != 0 {
		networkInterfaces.Netboot.Osie = &hardware.Hardware_Netboot_Osie{BaseUrl: c.Spec.ImageURL}
	}

	ip := &hardware.Hardware_DHCP_IP{}

	dhcpRequest := &hardware.Hardware_DHCP{
		Mac:      i.Spec.ManagementInterfaceMacAddress,
		Ip:       ip,
		Hostname: fmt.Sprintf("%s-%s", i.Name, i.Namespace),
	}

	ip.Address = i.Status.Address
	ip.Gateway = i.Status.Gateway
	ip.Netmask = i.Status.Netmask

	// update dhcp request
	networkInterfaces.Dhcp = dhcpRequest

	_, err = url.Parse(c.Spec.ConfigURL)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing server url")
	}

	mode := "join"
	if util.ConditionExists(i.Status.Conditions, bmaasv1alpha1.HarvesterCreateNode) {
		mode = "create"
	}

	m, err := generateMetaData(c.Spec.ConfigURL, c.Spec.HarvesterVersion, i.Spec.ManagementInterfaceMacAddress, mode,
		i.Spec.PrimaryDisk, c.Status.ClusterAddress, c.Status.ClusterToken, i.Status.GeneratedPassword, c.Spec.ClusterConfig.Nameservers, c.Spec.ClusterConfig.SSHKeys)
	if err != nil {
		return hw, errors.Wrap(err, "error during metadata generation")
	}
	hw = &hardware.Hardware{
		Id: i.Status.HardwareID,
		Network: &hardware.Hardware_Network{
			Interfaces: []*hardware.Hardware_Network_Interface{
				networkInterfaces,
			},
		},
		Metadata: m,
	}

	return hw, nil
}

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
