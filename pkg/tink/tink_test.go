package tink

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/util"
)

func Test_generateMetaDataV10(t *testing.T) {
	assert := require.New(t)
	m, err := generateMetaDataV10("http://localhost", "v1.0.1", "xx:xx:xx:xx:xx", "create",
		"/dev/sda", "192.168.1.100", "token", "password", "v1.0.2", []string{"8.8.8.8"}, []string{"abc"})
	assert.NoError(err, "no error should have occured")
	assert.Contains(m, "harvester.install.mode=create", "expected to find create mode in metadata")
	assert.Contains(m, "hwAddr:xx:xx:xx:xx:xx", "expected to find mac address in metadata")
	assert.NotContains(m, "scheme_version", "expected to not find scheme_version")
}

func Test_generateMetaDataV11(t *testing.T) {
	assert := require.New(t)
	m, err := generateMetaDataV11("http://localhost", "v1.0.1", "xx:xx:xx:xx:xx", "create",
		"/dev/sda", "192.168.1.100", "token", "password", "v1.0.2", []string{"8.8.8.8"}, []string{"abc"})
	assert.NoError(err, "no error should have occured")
	assert.Contains(m, "harvester.install.mode=create", "expected to find create mode in metadata")
	assert.Contains(m, "hwAddr:xx:xx:xx:xx:xx", "expected to find mac address in metadata")
	assert.Contains(m, "scheme_version", "expected to find scheme_version")
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
			HarvesterVersion: "v1.0.1",
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
)

func Test_GenerateHWRequestV10(t *testing.T) {
	assert := require.New(t)
	util.CreateOrUpdateCondition(i, seederv1alpha1.HarvesterCreateNode, "")
	hw, err := GenerateHWRequest(i, c)
	t.Log(i.Status)
	assert.NoError(err, "no error should occur during hardware generation")
	assert.Contains(hw.Spec.UserData, "harvester.install.mode=create", "expected to find create mode in metadata")
	assert.Contains(hw.Spec.UserData, "hwAddr:xx:xx:xx:xx:xx", "expected to find mac address in metadata")
	assert.Contains(hw.Spec.UserData, "dns_nameservers=8.8.8.8", "expected to find correct nameserver")
	assert.Contains(hw.Spec.UserData, "ssh_authorized_keys=\\\"- abc ", "expected to find ssh_keys")
	assert.Contains(hw.Spec.UserData, "token=token", "expected to find token")
	assert.Contains(hw.Spec.UserData, "password=password", "expected to find password")
	assert.Contains(hw.Spec.UserData, "harvester.install.vip=192.168.1.100", "expected to find a vip")
	assert.Contains(hw.Spec.UserData, "harvester.install.vip_mode=static", "expected to find vipMode static")
	assert.Equal(hw.Spec.Interfaces[0].DHCP.MAC, i.Spec.ManagementInterfaceMacAddress, "expected to find correct hardware address")
	assert.Equal(hw.Spec.Interfaces[0].DHCP.IP.Gateway, i.Status.Gateway, "expected to find correct gateway")
	assert.Equal(hw.Spec.Interfaces[0].DHCP.IP.Address, i.Status.Address, "expected to find correct address")
	assert.Equal(hw.Spec.Interfaces[0].DHCP.IP.Netmask, i.Status.Netmask, "expected to find correct netmask")
	assert.NotContains(hw.Spec.UserData, "scheme_version")
	assert.Contains(hw.Spec.UserData, "harvester.install.networks.harvester-mgmt", "expected to find harvester-mgmt in interfaces")
	assert.NotContains(hw.Spec.UserData, "harvester.install.management_interface", "expected to not find management_interface")
}

func Test_GenerateHWRequestV11(t *testing.T) {
	assert := require.New(t)
	clusterCopy := c.DeepCopy()
	clusterCopy.Spec.HarvesterVersion = "v1.1.0"
	util.CreateOrUpdateCondition(i, seederv1alpha1.HarvesterCreateNode, "")
	hw, err := GenerateHWRequest(i, clusterCopy)
	assert.NoError(err, "no error should occur during hardware generation")
	assert.Contains(hw.Spec.UserData, "harvester.install.mode=create", "expected to find create mode in metadata")
	assert.Contains(hw.Spec.UserData, "hwAddr:xx:xx:xx:xx:xx", "expected to find mac address in metadata")
	assert.Contains(hw.Spec.UserData, "dns_nameservers=8.8.8.8", "expected to find correct nameserver")
	assert.Contains(hw.Spec.UserData, "ssh_authorized_keys=\\\"- abc ", "expected to find ssh_keys")
	assert.Contains(hw.Spec.UserData, "token=token", "expected to find token")
	assert.Contains(hw.Spec.UserData, "password=password", "expected to find password")
	assert.Contains(hw.Spec.UserData, "harvester.install.vip=192.168.1.100", "expected to find a vip")
	assert.Contains(hw.Spec.UserData, "harvester.install.vip_mode=static", "expected to find vipMode static")
	assert.Equal(hw.Spec.Interfaces[0].DHCP.MAC, i.Spec.ManagementInterfaceMacAddress, "expected to find correct hardware address")
	assert.Equal(hw.Spec.Interfaces[0].DHCP.IP.Gateway, i.Status.Gateway, "expected to find correct gateway")
	assert.Equal(hw.Spec.Interfaces[0].DHCP.IP.Address, i.Status.Address, "expected to find correct address")
	assert.Equal(hw.Spec.Interfaces[0].DHCP.IP.Netmask, i.Status.Netmask, "expected to find correct netmask")
	assert.Contains(hw.Spec.UserData, "scheme_version")
	assert.NotContains(hw.Spec.UserData, "harvester.install.networks.harvester-mgmt", "expected to find harvester-mgmt in interfaces")
	assert.Contains(hw.Spec.Metadata.Instance.Userdata, "harvester.install.management_interface", "expected to not find management_interface")
}

func Test_GenerateHWRequestWithJoinV10(t *testing.T) {
	assert := require.New(t)
	util.RemoveCondition(i, seederv1alpha1.HarvesterCreateNode)
	hw, err := GenerateHWRequest(i, c)
	assert.NoError(err, "no error should occur during hardware generation")
	assert.Contains(hw.Spec.UserData, "harvester.server_url=https://192.168.1.100:8443", "expected to find join url")
}

func Test_GenerateHWRequestWithJoinV11(t *testing.T) {
	assert := require.New(t)
	util.RemoveCondition(i, seederv1alpha1.HarvesterCreateNode)
	clusterCopy := c.DeepCopy()
	clusterCopy.Spec.HarvesterVersion = "v1.1.0"
	hw, err := GenerateHWRequest(i, clusterCopy)
	assert.NoError(err, "no error should occur during hardware generation")
	assert.Contains(hw.Spec.UserData, "harvester.server_url=https://192.168.1.100", "expected to find join url")
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
					TemplateRef: seederv1alpha1.DefaultHarvesterProvisioningTemplate,
					HardwareRef: "test-node",
				},
			},
		}, {
			name: "custom workflow",
			i: &seederv1alpha1.Inventory{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-node",
					Namespace: "harvester-system",
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
				},
			},
		},
	}

	for _, v := range testCases {
		generatedWorkflow := GenerateWorkflow(v.i, v.c)
		assert.Equal(v.expectedWorkflow, generatedWorkflow, fmt.Sprintf("expected generatedWorkflow to match expected workflow for case %s", v.name))
	}
}
