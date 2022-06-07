package tink

import (
	bmaasv1alpha1 "github.com/harvester/bmaas/pkg/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func Test_generateMetaData(t *testing.T) {
	m, err := generateMetaData("http://localhost", "v1.0.1", "xx:xx:xx:xx:xx", "create",
		"/dev/sda", "192.168.1.100", "token", "password", []string{"8.8.8.8"}, []string{"abc"})
	assert.NoError(t, err, "no error should have occured")
	assert.Contains(t, m, "harvester.install.mode=create", "expected to find create mode in metadata")
	assert.Contains(t, m, "hwAddr:xx:xx:xx:xx:xx", "expected to find mac address in metadata")
	assert.Contains(t, m, "\"slug\":\"v1.0.1\"", "expected find a slug in metadata")
}

func Test_GenerateHWRequest(t *testing.T) {
	i := &bmaasv1alpha1.Inventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "firstnode",
			Namespace: "default",
		},
		Spec: bmaasv1alpha1.InventorySpec{
			PrimaryDisk:                   "/dev/sda",
			ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
			BaseboardManagementSpec: rufio.BaseboardManagementSpec{
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
		Status: bmaasv1alpha1.InventoryStatus{
			Status:            bmaasv1alpha1.InventoryReady,
			GeneratedPassword: "password",
			HardwareID:        "uuid",
			Conditions: []bmaasv1alpha1.Conditions{
				{
					Type:      bmaasv1alpha1.HarvesterCreateNode,
					StartTime: metav1.Now(),
				},
			},
			Cluster: bmaasv1alpha1.ObjectReference{
				Name:      "harvester-one",
				Namespace: "default",
			},
			PXEBootInterface: bmaasv1alpha1.PXEBootInterface{
				Address:     "192.168.1.129",
				Netmask:     "255.255.255.0",
				Gateway:     "192.168.1.1",
				NameServers: []string{"8.8.8.8", "8.8.4.4"},
			},
		},
	}

	c := &bmaasv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "harvester-one",
			Namespace: "default",
		},
		Spec: bmaasv1alpha1.ClusterSpec{
			HarvesterVersion: "v1.0.1",
			VIPConfig: bmaasv1alpha1.VIPConfig{
				AddressPoolReference: bmaasv1alpha1.ObjectReference{
					Name:      "management-pool",
					Namespace: "default",
				},
				StaticAddress: "192.168.1.100",
			},
			Nodes: []bmaasv1alpha1.NodeConfig{
				{
					InventoryReference: bmaasv1alpha1.ObjectReference{
						Name:      "firstnode",
						Namespace: "default",
					},
					AddressPoolReference: bmaasv1alpha1.ObjectReference{
						Name:      "management-pool",
						Namespace: "default",
					},
				},
			},
			ClusterConfig: bmaasv1alpha1.ClusterConfig{
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
		Status: bmaasv1alpha1.ClusterStatus{
			ClusterToken:   "token",
			ClusterAddress: "192.168.1.100",
		},
	}

	hw, err := GenerateHWRequest(i, c)
	assert.NoError(t, err, "no error should occur during hardware generation")
	assert.Contains(t, hw.Spec.Metadata.Instance.Userdata, "harvester.install.mode=create", "expected to find create mode in metadata")
	assert.Contains(t, hw.Spec.Metadata.Instance.Userdata, "hwAddr:xx:xx:xx:xx:xx", "expected to find mac address in metadata")
	assert.Contains(t, hw.Spec.Metadata.Instance.Userdata, "\"slug\":\"v1.0.1\"", "expected find a slug in metadata")
	assert.Contains(t, hw.Spec.Metadata.Instance.Userdata, "dnsNameservers=8.8.8.8", "expected to find correct nameserver")
	assert.Contains(t, hw.Spec.Metadata.Instance.Userdata, "ssh_authorized_keys=\\\"- abc ", "expected to find ssh_keys")
	assert.Contains(t, hw.Spec.Metadata.Instance.Userdata, "token=token", "expected to find token")
	assert.Contains(t, hw.Spec.Metadata.Instance.Userdata, "password=password", "expected to find password")
	assert.Contains(t, hw.Spec.Metadata.Instance.Userdata, "harvester.install.vip=192.168.1.100", "expected to find a vip")
	assert.Contains(t, hw.Spec.Metadata.Instance.Userdata, "harvester.install.vipMode=static", "expected to find vipMode static")
	assert.Equal(t, hw.Spec.Metadata.Instance.ID, i.Status.HardwareID, "expected to find correct hardware uuid")
	assert.Equal(t, hw.Spec.Interfaces[0].DHCP.MAC, i.Spec.ManagementInterfaceMacAddress, "expected to find correct hardware address")
	assert.Equal(t, hw.Spec.Interfaces[0].DHCP.IP.Gateway, i.Status.Gateway, "expected to find correct gateway")
	assert.Equal(t, hw.Spec.Interfaces[0].DHCP.IP.Address, i.Status.Address, "expected to find correct address")
	assert.Equal(t, hw.Spec.Interfaces[0].DHCP.IP.Netmask, i.Status.Netmask, "expected to find correct netmask")
}
