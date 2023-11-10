package mock

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FakeWorkflowReconciler implements a fake reconcile loop for workflow integration testing
type FakeWorkflowReconciler struct {
	client.Client
	logr.Logger
	Scheme *runtime.Scheme
}

func (f *FakeWorkflowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	w := &tinkv1alpha1.Workflow{}
	f.Info("Reconcilling workflow job objects", req.Name, req.Namespace)
	err := f.Get(ctx, req.NamespacedName, w)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		f.Error(err, "error fetching baseboard object")
		return ctrl.Result{}, err
	}

	// state is already patched nothing else to do
	if w.Status.State != "" {
		return ctrl.Result{}, nil
	}

	if strings.Contains(w.Name, "fail") {
		w.Status.State = tinkv1alpha1.WorkflowStateFailed
	} else {
		w.Status.State = tinkv1alpha1.WorkflowStateSuccess
	}

	return ctrl.Result{}, f.Status().Update(ctx, w)

}

// SetupWithManager sets up the controller with the Manager.
func (f *FakeWorkflowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&tinkv1alpha1.Workflow{}).
		Complete(f)
}
