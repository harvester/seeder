package util

import (
	"fmt"

	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

// GenerateJob will generate a power action rufio job for an inventory object
func GenerateJob(name, namespace, powerAction string) *rufio.Job {
	var tasks []rufio.Action
	powerOffTask := rufio.Action{
		PowerAction: rufio.PowerHardOff.Ptr(),
	}
	powerOnTask := rufio.Action{
		PowerAction: rufio.PowerOn.Ptr(),
	}

	pxeBoot := rufio.Action{
		OneTimeBootDeviceAction: &rufio.OneTimeBootDeviceAction{
			Devices: []rufio.BootDevice{
				rufio.PXE,
			},
		},
	}

	switch powerAction {
	case seederv1alpha1.NodePowerActionPowerOn:
		tasks = append(tasks, powerOnTask)
	case seederv1alpha1.NodePowerActionShutdown:
		tasks = append(tasks, powerOffTask)
	case seederv1alpha1.NodePowerActionReboot:
		tasks = append(tasks, powerOffTask, pxeBoot, powerOnTask)
	default:
		return nil
	}

	return &rufio.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-%s-", name, powerAction),
			Namespace:    namespace,
			Labels: map[string]string{
				"inventory.metal.harvesterhci.io": name,
			},
		},
		Spec: rufio.JobSpec{
			MachineRef: rufio.MachineRef{
				Name:      name,
				Namespace: namespace,
			},
			Tasks: tasks,
		},
	}
}
