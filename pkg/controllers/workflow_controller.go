package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

// WorkflowReconciler reconciles a Workflow object
type WorkflowReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logr.Logger
	record.EventRecorder
}

/* Aim of workflow controller is to watch the workflow objects and disable ipxe/workflow options on hardware
object. This is needed as the default workflow reboots harvester post install, and this change is needed to ensure
that the inventory object is not stuck in an installation loop*/

func (r *WorkflowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Info("reconcilling workflow object", req.Name, req.Namespace)
	wObj := &tinkv1alpha1.Workflow{}
	err := r.Get(ctx, req.NamespacedName, wObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.Error(err, "unable to fetch workflow object")
		return ctrl.Result{}, err
	}

	if wObj.DeletionTimestamp != nil {
		// no further action is needed if workflow is being deleted
		return ctrl.Result{}, nil
	}

	r.Info(fmt.Sprintf("current workflow status: %v", wObj.Status), req.Name, req.Namespace)

	hw := &tinkv1alpha1.Hardware{}
	err = r.Get(ctx, req.NamespacedName, hw)
	if err != nil {
		// for now we return error and requeue workflow
		// this includes case when hardware object no longer exists
		// may need to fine tune to handle hardware deletion
		return ctrl.Result{}, err
	}

	// enable/disable IPXE/Workflow based on workflow status
	if wObj.Status.State == tinkv1alpha1.WorkflowStateSuccess && (*hw.Spec.Interfaces[0].Netboot.AllowWorkflow || *hw.Spec.Interfaces[0].Netboot.AllowPXE) {

		hw.Spec.Interfaces[0].Netboot.AllowWorkflow = &[]bool{false}[0]
		hw.Spec.Interfaces[0].Netboot.AllowPXE = &[]bool{false}[0]

		return ctrl.Result{}, r.Update(ctx, hw)
	}

	cluster, err := r.getOwnerCluster(ctx, wObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("error fetching parent cluster object for workflow %s: %v", wObj.Name, err)
	}

	if cluster == nil {
		return ctrl.Result{}, nil
	}

	if wObj.Status.State == tinkv1alpha1.WorkflowStateSuccess {
		r.EventRecorder.Event(cluster, "Normal", seederv1alpha1.WorkflowLoggerName, fmt.Sprintf("workflow %s completed successfully", wObj.Name))
	}
	if wObj.Status.State == tinkv1alpha1.WorkflowStateFailed {
		for _, task := range wObj.Status.Tasks {
			for _, action := range task.Actions {
				if action.Status == tinkv1alpha1.WorkflowStateFailed {
					r.EventRecorder.Event(cluster, "Warning", seederv1alpha1.WorkflowLoggerName, fmt.Sprintf("workflow %s failed for task %s, action %s", wObj.Name, task.Name, action.Name))
				}
			}
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkflowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&tinkv1alpha1.Workflow{}).
		Complete(r)
}

func (r *WorkflowReconciler) getOwnerCluster(ctx context.Context, wf *tinkv1alpha1.Workflow) (*seederv1alpha1.Cluster, error) {
	owners := wf.GetOwnerReferences()
	clusterObj := &seederv1alpha1.Cluster{}
	for _, v := range owners {
		r.Info("owners are", v.Name, wf.Namespace)
		if v.Kind == "Cluster" {
			err := r.Get(ctx, types.NamespacedName{Name: v.Name, Namespace: wf.Namespace}, clusterObj)
			return clusterObj, err
		}
	}
	return nil, nil
}
