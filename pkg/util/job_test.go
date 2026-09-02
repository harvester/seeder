package util

import (
	"testing"

	"github.com/stretchr/testify/require"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

// Test_GenerateJob_Reboot ensures the reboot job's one-time PXE boot task
// always requests EFI boot, regardless of the machine's current firmware
// mode. See https://github.com/harvester/harvester/issues/11562: BMC
// implementations that strictly honour the EFIBoot flag (e.g. KubeVirtBMC
// v0.10.0+) fall back to BIOS mode when it is left unset, which the
// Harvester installer rejects.
func Test_GenerateJob_Reboot(t *testing.T) {
	assert := require.New(t)

	j := GenerateJob("node1", "default", seederv1alpha1.NodePowerActionReboot)
	assert.NotNil(j, "expected a job to be generated for reboot action")
	assert.Len(j.Spec.Tasks, 3, "expected reboot job to contain 3 tasks")

	assert.NotNil(j.Spec.Tasks[0].PowerAction, "expected first task to be a power action")
	assert.Equal(rufio.PowerHardOff, *j.Spec.Tasks[0].PowerAction, "expected first task to power off the machine")

	bootTask := j.Spec.Tasks[1].OneTimeBootDeviceAction
	assert.NotNil(bootTask, "expected second task to be a one-time boot device action")
	assert.Equal([]rufio.BootDevice{rufio.PXE}, bootTask.Devices, "expected boot device to be pxe")
	assert.True(bootTask.EFIBoot, "expected one-time PXE boot task to request EFI boot")

	assert.NotNil(j.Spec.Tasks[2].PowerAction, "expected third task to be a power action")
	assert.Equal(rufio.PowerOn, *j.Spec.Tasks[2].PowerAction, "expected third task to power on the machine")
}
