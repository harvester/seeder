package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

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
					AuthSecretRef: corev1.SecretReference{
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
			Conditions: []seederv1alpha1.Conditions{
				{
					Type: seederv1alpha1.HarvesterCreateNode,
				},
			},
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
)

func Test_ConditionsExist(t *testing.T) {
	assert := require.New(t)
	CreateOrUpdateCondition(i, seederv1alpha1.BMCObjectCreated, "BMCObject created")
	ok := ConditionExists(i, seederv1alpha1.BMCObjectCreated)
	assert.True(ok, "expected condition to be found")
}

func Test_ConditionsExist_False(t *testing.T) {
	assert := require.New(t)
	CreateOrUpdateCondition(i, seederv1alpha1.BMCObjectCreated, "BMCObject created")
	ok := ConditionExists(i, seederv1alpha1.BMCJobSubmitted)
	assert.False(ok, "expected condition to be not found")
}

func Test_RemoveCondition(t *testing.T) {
	assert := require.New(t)
	CreateOrUpdateCondition(i, seederv1alpha1.BMCObjectCreated, "BMC Object created")
	ok := ConditionExists(i, seederv1alpha1.BMCObjectCreated)
	assert.True(ok, "expected new condition to be present")
	RemoveCondition(i, seederv1alpha1.BMCObjectCreated)
	ok = ConditionExists(i, seederv1alpha1.BMCObjectCreated)
	assert.False(ok, "expected condition to be not found")
}

func Test_AddCondition(t *testing.T) {
	assert := require.New(t)
	CreateOrUpdateCondition(i, seederv1alpha1.BMCObjectCreated, "BMCObject created")
	ok := ConditionExists(i, seederv1alpha1.BMCObjectCreated)
	assert.True(ok, "expected new condition to be present")
	CreateOrUpdateCondition(i, seederv1alpha1.BMCJobComplete, "BMCObject created")
	ok = ConditionExists(i, seederv1alpha1.BMCJobComplete)
	assert.True(ok, "expected new condition to be present")
	ok = ConditionExists(i, seederv1alpha1.BMCObjectCreated)
	assert.True(ok, "expected original condition to be present", i)
}

func Test_UpdateCondition(t *testing.T) {
	assert := require.New(t)
	CreateOrUpdateCondition(i, seederv1alpha1.BMCObjectCreated, "BMCObject Created")
	orgTime := seederv1alpha1.BMCObjectCreated.GetLastUpdated(i)
	time.Sleep(5 * time.Second)
	CreateOrUpdateCondition(i, seederv1alpha1.BMCObjectCreated, "BMCObject Created")
	ok := ConditionExists(i, seederv1alpha1.BMCObjectCreated)
	newTime := seederv1alpha1.BMCObjectCreated.GetLastUpdated(i)
	t.Log(orgTime)
	t.Log(newTime)
	assert.True(ok, "expected condition to be present")
	assert.NotEqual(orgTime, newTime, "original time should be unchanged")
}

func Test_ErrorCondition(t *testing.T) {
	assert := require.New(t)
	SetErrorCondition(i, seederv1alpha1.MachineNotContactable, "machine not reachable")
	assert.True(ConditionExists(i, seederv1alpha1.MachineNotContactable), "expected to find condition MachineNotContactable")
}
