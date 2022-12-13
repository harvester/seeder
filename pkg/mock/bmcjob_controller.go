package mock

import (
	"context"

	"github.com/go-logr/logr"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FakeBaseboardJobReconciller implements a fake reconcile loop for integration testing
type FakeBaseboardJobReconciller struct {
	client.Client
	logr.Logger
	Scheme *runtime.Scheme
}

func (f *FakeBaseboardJobReconciller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	b := &rufio.Job{}
	f.Info("Reconcilling baseboard job objects", req.Name, req.Namespace)
	err := f.Get(ctx, req.NamespacedName, b)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		f.Error(err, "error fetching baseboard object")
		return ctrl.Result{}, err
	}

	// patch power status to mimic real world action
	b.SetCondition(rufio.JobCompleted, rufio.ConditionTrue)
	return ctrl.Result{}, f.Status().Update(ctx, b)
}

// SetupWithManager sets up the controller with the Manager.
func (f *FakeBaseboardJobReconciller) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rufio.Job{}).
		Complete(f)
}
