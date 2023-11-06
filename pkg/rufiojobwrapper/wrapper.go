package rufiojobwrapper

import (
	"context"

	rufiocontrollers "github.com/tinkerbell/rufio/controller"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The wrapper only exists because the method signature for SetupManager in job controller is not in line with the
// Machine and Task controllers.
// This can be removed or enhanced based on how the rest of the project evolves
// https://github.com/tinkerbell/rufio/blob/main/controllers/job_controller.go
type RufioJobWrapper struct {
	*rufiocontrollers.JobReconciler
	context.Context
}

func NewRufioWrapper(ctx context.Context, client client.Client) *RufioJobWrapper {
	return &RufioJobWrapper{
		rufiocontrollers.NewJobReconciler(client),
		ctx,
	}
}

func (r *RufioJobWrapper) SetupWithManager(mgr ctrl.Manager) error {
	return r.JobReconciler.SetupWithManager(r.Context, mgr)
}
