package tink

import (
	"fmt"
	"testing"

	"github.com/rancher/wrangler/pkg/yaml"
	"github.com/stretchr/testify/require"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester-installer/pkg/config"
	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/util"
)

func Test_createModeCloudConfig(t *testing.T) {
	assert := require.New(t)
	cloudConfig, err := generateCloudConfig("http://endpoint/node.yaml", "ab:cd:ef:gh:ij:kl", "create", "192.168.1.100", "token", "password", "192.168.1.101", "255.255.255.0", "192.168.1.1", []string{"8.8.8.8"}, []string{"ssh-key 1", "ssh-key 2"}, nil, "http://imagestore/iso", "v1.2.1", "http://seeder-endpoint", "sample", "harvester-system")
	assert.NoError(err)
	hc := config.NewHarvesterConfig()
	err = yaml.Unmarshal([]byte(cloudConfig), hc)
	assert.NoError(err)
	assert.True(hc.Install.Automatic, "expected automatic installation to be set")
	assert.Empty(hc.ServerURL, "expected serverURL to be empty")
	assert.NotEmpty(hc.Install.Vip, "expected VIP to be set")
	assert.Equal(hc.Install.VipMode, "static", "expected vip mode to be static")
	assert.Equal(hc.Install.Mode, "create", "expected install mode to be create")
	assert.Len(hc.Install.ManagementInterface.Interfaces, 1, "expected to find 1 interface defined")
	assert.NotEmpty(hc.Install.ConfigURL, "expected configURL to be set")
	assert.NotEmpty(hc.OS.Password, "expected password to be set")
	assert.Len(hc.OS.DNSNameservers, 1, "expected to find 1 dns server")
	assert.Len(hc.OS.SSHAuthorizedKeys, 2, "expected to find 2 ssh keys specified")
	assert.NotEmpty(hc.Install.ManagementInterface.IP, "expected IP to be set")
	assert.NotEmpty(hc.Install.ManagementInterface.Gateway, "expected gateway to be set")
	assert.NotEmpty(hc.Install.ManagementInterface.SubnetMask, "expected subnet mask to be set")
}

func Test_joinModeCloudConfig(t *testing.T) {
	assert := require.New(t)
	cloudConfig, err := generateCloudConfig("http://endpoint/node.yaml", "ab:cd:ef:gh:ij:kl", "join", "192.168.1.100", "token", "password", "192.168.1.101", "255.255.255.0", "192.168.1.1", []string{"8.8.8.8"}, []string{"ssh-key 1", "ssh-key 2"}, nil, "http://imagestore/iso", "v1.2.1", "http://seeder-endpoint", "sample", "harvester-system")
	assert.NoError(err)
	hc := config.NewHarvesterConfig()
	err = yaml.Unmarshal([]byte(cloudConfig), hc)
	assert.NoError(err)
	assert.True(hc.Install.Automatic, "expected automatic installation to be set")
	assert.NotEmpty(hc.ServerURL, "expected serverURL to be empty")
	assert.Equal(hc.Install.Mode, "join", "expected install mode to be create")
	assert.Len(hc.Install.ManagementInterface.Interfaces, 1, "expected to find 1 interface defined")
	assert.NotEmpty(hc.Install.ConfigURL, "expected configURL to be set")
	assert.NotEmpty(hc.OS.Password, "expected password to be set")
	assert.Len(hc.OS.DNSNameservers, 1, "expected to find 1 dns server")
	assert.Len(hc.OS.SSHAuthorizedKeys, 2, "expected to find 2 ssh keys specified")
	assert.NotEmpty(hc.Install.ManagementInterface.IP, "expected IP to be set")
	assert.NotEmpty(hc.Install.ManagementInterface.Gateway, "expected gateway to be set")
	assert.NotEmpty(hc.Install.ManagementInterface.SubnetMask, "expected subnet mask to be set")
}

var (
	i = &seederv1alpha1.Inventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "firstnode",
			Namespace: "default",
		},
		Spec: seederv1alpha1.InventorySpec{
			PrimaryDisk:                   "/dev/sda",
			ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
			BaseboardManagementSpec: rufio.MachineSpec{
				Connection: rufio.Connection{
					Host: "localhost",
					Port: 623,
					AuthSecretRef: v1.SecretReference{
						Name:      "firstnode",
						Namespace: "default",
					},
					InsecureTLS: true,
				},
			},
		},
		Status: seederv1alpha1.InventoryStatus{
			Status:            seederv1alpha1.InventoryReady,
			GeneratedPassword: "password",
			HardwareID:        "uuid",
			Cluster: seederv1alpha1.ObjectReference{
				Name:      "harvester-one",
				Namespace: "default",
			},
			PXEBootInterface: seederv1alpha1.PXEBootInterface{
				Address:     "192.168.1.129",
				Netmask:     "255.255.255.0",
				Gateway:     "192.168.1.1",
				NameServers: []string{"8.8.8.8", "8.8.4.4"},
			},
		},
	}

	c = &seederv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "harvester-one",
			Namespace: "default",
		},
		Spec: seederv1alpha1.ClusterSpec{
			HarvesterVersion: "v1.2.0",
			VIPConfig: seederv1alpha1.VIPConfig{
				AddressPoolReference: seederv1alpha1.ObjectReference{
					Name:      "management-pool",
					Namespace: "default",
				},
				StaticAddress: "192.168.1.100",
			},
			Nodes: []seederv1alpha1.NodeConfig{
				{
					InventoryReference: seederv1alpha1.ObjectReference{
						Name:      "firstnode",
						Namespace: "default",
					},
					AddressPoolReference: seederv1alpha1.ObjectReference{
						Name:      "management-pool",
						Namespace: "default",
					},
				},
			},
			ClusterConfig: seederv1alpha1.ClusterConfig{
				SSHKeys: []string{
					"abc",
					"def",
				},
				Nameservers: []string{
					"8.8.8.8",
					"8.8.4.4",
				},
				ConfigURL: "http://endpoint",
			},
		},
		Status: seederv1alpha1.ClusterStatus{
			ClusterToken:   "token",
			ClusterAddress: "192.168.1.100",
		},
	}

	svc = &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "harvester-system",
		},
		Spec: v1.ServiceSpec{},
		Status: v1.ServiceStatus{
			LoadBalancer: v1.LoadBalancerStatus{
				Ingress: []v1.LoadBalancerIngress{
					{
						IP: "127.0.0.1",
					},
				},
			},
		},
	}

	hegelSvc = &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hegel-svc",
			Namespace: "harvester-system",
		},
		Spec: v1.ServiceSpec{},
		Status: v1.ServiceStatus{
			LoadBalancer: v1.LoadBalancerStatus{
				Ingress: []v1.LoadBalancerIngress{
					{
						IP: "127.0.0.1",
					},
				},
			},
		},
	}
)

func Test_GenerateHWRequest(t *testing.T) {
	assert := require.New(t)
	util.CreateOrUpdateCondition(i, seederv1alpha1.HarvesterCreateNode, "")
	hw, err := GenerateHWRequest(i, c, svc, hegelSvc)
	assert.NoError(err, "expected no error during hardware generation")
	assert.NotNil(hw.Spec.UserData, "expected user data to be set")
}

func Test_GenerateWorkflow(t *testing.T) {
	assert := require.New(t)
	var testCases = []struct {
		name             string
		i                *seederv1alpha1.Inventory
		c                *seederv1alpha1.Cluster
		expectedWorkflow *tinkv1alpha1.Workflow
	}{
		{
			name: "default workflow",
			i: &seederv1alpha1.Inventory{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-node",
					Namespace: "harvester-system",
				},
				Spec: seederv1alpha1.InventorySpec{
					ManagementInterfaceMacAddress: "xx:xx",
				},
			},
			c: &seederv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "harvester-system",
				},
			},
			expectedWorkflow: &tinkv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-node",
					Namespace: "harvester-system",
				},
				Spec: tinkv1alpha1.WorkflowSpec{
					TemplateRef: "test-node",
					HardwareRef: "test-node",
					HardwareMap: map[string]string{
						"device_1": "xx:xx",
					},
				},
			},
		}, {
			name: "custom workflow",
			i: &seederv1alpha1.Inventory{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-node",
					Namespace: "harvester-system",
				},
				Spec: seederv1alpha1.InventorySpec{
					ManagementInterfaceMacAddress: "xx:xx",
				},
			},
			c: &seederv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "harvester-system",
				},
				Spec: seederv1alpha1.ClusterSpec{
					ClusterConfig: seederv1alpha1.ClusterConfig{
						CustomProvisioningTemplate: "override-template",
					},
				},
			},
			expectedWorkflow: &tinkv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-node",
					Namespace: "harvester-system",
				},
				Spec: tinkv1alpha1.WorkflowSpec{
					TemplateRef: "override-template",
					HardwareRef: "test-node",
					HardwareMap: map[string]string{
						"device_1": "xx:xx",
					},
				},
			},
		},
	}

	for _, v := range testCases {
		generatedWorkflow := GenerateWorkflow(v.i, v.c)
		assert.Equal(v.expectedWorkflow, generatedWorkflow, fmt.Sprintf("expected generatedWorkflow to match expected workflow for case %s", v.name))
	}
}

func Test_generateIPXEScript(t *testing.T) {
	assert := require.New(t)
	output, err := generateIPXEScript("v1.1.3", "http://imagestore/iso", "hegelEndpoint", "ab:cd:ef:gh:ij", "/dev/sda", "172.19.108.2", "255.255.255.0", "172.19.108.1")
	assert.NoError(err, "expect no error during generation of ipxe script")
	assert.Contains(output, "harvester.install.management_interface.method=static", "expected to find static interface configiration")
	assert.Contains(output, "harvester.install.management_interface.ip", "expected to find an ip for management interface")
	assert.Contains(output, "harvester.install.management_interface.subnet_mask", "expected to find subnet mask for management interface")
	assert.Contains(output, "harvester.install.management_interface.gateway", "expected to find gateway for management interface")
	assert.Contains(output, "harvester.install.device", "expected to find install disk")
}

func Test_GenerateHardwareRequestV11(t *testing.T) {
	assert := require.New(t)
	cObj := c.DeepCopy()
	cObj.Spec.HarvesterVersion = "v1.1.2"
	hw, err := GenerateHWRequest(i, cObj, svc, hegelSvc)
	assert.NoError(err, "expected no error during hardware generation")
	assert.NotNil(hw.Spec.UserData, "expected user data to be set")
	for _, v := range hw.Spec.Interfaces {
		assert.NotNil(v.Netboot.IPXE, "expect ipxe definition to exist")
		assert.NotEmpty(v.Netboot.IPXE.Contents, "expected content script to be defined")
	}
}

func Test_createModeCloudConfigV11(t *testing.T) {
	assert := require.New(t)
	cloudConfig, err := generateCloudConfig("http://endpoint/node.yaml", "ab:cd:ef:gh:ij:kl", "create", "192.168.1.100", "token", "password", "192.168.1.101", "255.255.255.0", "192.168.1.1", []string{"8.8.8.8"}, []string{"ssh-key 1", "ssh-key 2"}, nil, "http://imagestore/iso", "v1.1.2", "http://seeder-endpoint", "sample", "harvester-system")
	assert.NoError(err)
	hc := config.NewHarvesterConfig()
	err = yaml.Unmarshal([]byte(cloudConfig), hc)
	assert.NoError(err)
	assert.True(hc.Install.Automatic, "expected automatic installation to be set")
	assert.Empty(hc.ServerURL, "expected serverURL to be empty")
	assert.NotEmpty(hc.Install.Vip, "expected VIP to be set")
	assert.Equal(hc.Install.VipMode, "static", "expected vip mode to be static")
	assert.Equal(hc.Install.Mode, "create", "expected install mode to be create")
	assert.Len(hc.Install.ManagementInterface.Interfaces, 1, "expected to find 1 interface defined")
	assert.Empty(hc.Install.ConfigURL, "expected configURL to be empty")
	assert.NotEmpty(hc.OS.Password, "expected password to be set")
	assert.Len(hc.OS.DNSNameservers, 1, "expected to find 1 dns server")
	assert.Len(hc.OS.SSHAuthorizedKeys, 2, "expected to find 2 ssh keys specified")
	assert.NotEmpty(hc.Install.ManagementInterface.IP, "expected IP to be set")
	assert.NotEmpty(hc.Install.ManagementInterface.Gateway, "expected gateway to be set")
	assert.NotEmpty(hc.Install.ManagementInterface.SubnetMask, "expected subnet mask to be set")
	assert.Len(hc.Install.Webhooks, 1, "expected to find atleast 1 webhook definition")
}

func Test_joinModeCloudConfigV11(t *testing.T) {
	assert := require.New(t)
	cloudConfig, err := generateCloudConfig("http://endpoint/node.yaml", "ab:cd:ef:gh:ij:kl", "join", "192.168.1.100", "token", "password", "192.168.1.101", "255.255.255.0", "192.168.1.1", []string{"8.8.8.8"}, []string{"ssh-key 1", "ssh-key 2"}, nil, "http://imagestore/iso", "v1.1.2", "http://seeder-endpoint", "sample", "harvester-system")
	assert.NoError(err)
	hc := config.NewHarvesterConfig()
	err = yaml.Unmarshal([]byte(cloudConfig), hc)
	assert.NoError(err)
	assert.True(hc.Install.Automatic, "expected automatic installation to be set")
	assert.NotEmpty(hc.ServerURL, "expected serverURL to be empty")
	assert.Equal(hc.Install.Mode, "join", "expected install mode to be create")
	assert.Len(hc.Install.ManagementInterface.Interfaces, 1, "expected to find 1 interface defined")
	assert.Empty(hc.Install.ConfigURL, "expected configURL to be empty")
	assert.NotEmpty(hc.OS.Password, "expected password to be set")
	assert.Len(hc.OS.DNSNameservers, 1, "expected to find 1 dns server")
	assert.Len(hc.OS.SSHAuthorizedKeys, 2, "expected to find 2 ssh keys specified")
	assert.NotEmpty(hc.Install.ManagementInterface.IP, "expected IP to be set")
	assert.NotEmpty(hc.Install.ManagementInterface.Gateway, "expected gateway to be set")
	assert.NotEmpty(hc.Install.ManagementInterface.SubnetMask, "expected subnet mask to be set")
	assert.Len(hc.Install.Webhooks, 1, "expected to find atleast 1 webhook definition")
}
