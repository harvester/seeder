package controllers

import (
	"fmt"

	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

func generateJob(name, namespace, powerAction string) *rufio.Job {
	var tasks []rufio.Action
	powerOffTask := rufio.Action{
		PowerAction: rufio.PowerHardOff.Ptr(),
	}
	powerOnTask := rufio.Action{
		PowerAction: rufio.PowerOn.Ptr(),
	}

	switch powerAction {
	case seederv1alpha1.NodePowerActionPowerOn:
		tasks = append(tasks, powerOnTask)
	case seederv1alpha1.NodePowerActionShutdown:
		tasks = append(tasks, powerOffTask)
	case seederv1alpha1.NodePowerActionReboot:
		tasks = append(tasks, powerOffTask, powerOnTask)
	default:
		return nil
	}

	return &rufio.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-%s", name, powerAction),
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
