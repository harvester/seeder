package tink

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/url"
	"strings"

	bmaasv1alpha1 "github.com/harvester/bmaas/pkg/api/v1alpha1"
	"github.com/pkg/errors"
	hw "github.com/tinkerbell/tink/client"
	"github.com/tinkerbell/tink/protos/hardware"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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

	// Specify non default location to load ISO's
	if len(regoReq.Spec.ImageURL) != 0 {
		networkInterfaces.Netboot.Osie = &hardware.Hardware_Netboot_Osie{BaseUrl: regoReq.Spec.ImageURL}
	}

	ip := &hardware.Hardware_DHCP_IP{}

	dhcpRequest := &hardware.Hardware_DHCP{
		Mac:      regoReq.Spec.MacAddress,
		Ip:       ip,
		Hostname: regoReq.Name,
	}

	if len(regoReq.Spec.Address) != 0 && len(regoReq.Spec.Gateway) != 0 && len(regoReq.Spec.Netmask) != 0 {
		ip.Address = regoReq.Spec.Address
		ip.Gateway = regoReq.Spec.Gateway
		ip.Netmask = regoReq.Spec.Netmask

	}

	// update dhcp request
	networkInterfaces.Dhcp = dhcpRequest

	url, err := url.Parse(serverURL)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing server url")
	}

	urlArr := strings.Split(url.Host, ":")

	m, err := generateMetaData(regoReq, urlArr[0])
	if err != nil {
		return hw, errors.Wrap(err, "error during metadata generation")
	}
	hw = &hardware.Hardware{
		Id: regoReq.Status.UUID,
		Network: &hardware.Hardware_Network{
			Interfaces: []*hardware.Hardware_Network_Interface{
				networkInterfaces,
			},
		},
		Metadata: m,
	}

	return hw, nil
}

func generateMetaData(regoReq *nodev1alpha1.Register, serverURL string) (metadata string, err error) {

	var tmpStruct struct {
		ServerUrl     string
		DefaultPort   string
		UUID          string
		Slug          string
		Interface     string
		BootArguments string
	}
	var output bytes.Buffer
	tmpStruct.ServerUrl = serverURL
	tmpStruct.DefaultPort = nodev1alpha1.DefaultConfigURLPort
	tmpStruct.UUID = regoReq.Status.UUID
	if regoReq.Spec.Slug != "" {
		tmpStruct.Slug = regoReq.Spec.Slug
	} else {
		tmpStruct.Slug = nodev1alpha1.DefaultSlug
	}

	tmpStruct.Interface = regoReq.Spec.Interface
	tmpStruct.BootArguments = regoReq.Spec.KernelBootArguments
	var metaDataStruct = `{"facility":{"facility_code":"onprem"},"instance":{"userdata":"harvester.install.config_url=http://{{ .ServerUrl }}:{{ .DefaultPort }}/config/{{ .UUID }} {{ .BootArguments }}" ,"operating_system":{"slug":"{{ .Slug }}"}}}`

	metadataTmpl := template.Must(template.New("MetData").Parse(metaDataStruct))

	err = metadataTmpl.Execute(&output, tmpStruct)

	if err != nil {
		return metadata, err
	}

	metadata = output.String()
	return metadata, nil
}
