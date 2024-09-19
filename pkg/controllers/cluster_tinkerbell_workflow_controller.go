package controllers

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"
	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/tink"
	"github.com/harvester/seeder/pkg/util"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ClusterTinkerbellWorkflowReconciler reconciles a Cluster object and watches associated TinkerbellTemplate for changes
type ClusterTinkerbellWorkflowReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logr.Logger
}

func (r *ClusterTinkerbellWorkflowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Info("Reconcilling inventory objects", req.Name, req.Namespace)
	// TODO(user): your logic here
	cObj := &seederv1alpha1.Cluster{}

	err := r.Get(ctx, req.NamespacedName, cObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.Error(err, "unable to fetch cluster object")
		return ctrl.Result{}, err
	}

	c := cObj.DeepCopy()

	// ignore the local cluster
	if c.Name == seederv1alpha1.DefaultLocalClusterName && c.Namespace == seederv1alpha1.DefaultLocalClusterNamespace {
		return ctrl.Result{}, nil
	}

	reconcileList := []clusterReconciler{
		r.createTinkerbellWorkflow,
	}

	if c.DeletionTimestamp.IsZero() {
		for _, reconciler := range reconcileList {
			if err := reconciler(ctx, c); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// createTinkerbellWorkflow will create workflow objects for all nodes in the cluster
func (r *ClusterTinkerbellWorkflowReconciler) createTinkerbellWorkflow(ctx context.Context, cObj *seederv1alpha1.Cluster) error {

	c := cObj.DeepCopy()
	if c.Status.Status == seederv1alpha1.ClusterNodesPatched || c.Status.Status == seederv1alpha1.ClusterTinkHardwareSubmitted || c.Status.Status == seederv1alpha1.ClusterRunning {
		for _, i := range c.Spec.Nodes {
			inventory := &seederv1alpha1.Inventory{}
			err := r.Get(ctx, types.NamespacedName{Namespace: i.InventoryReference.Namespace, Name: i.InventoryReference.Name}, inventory)
			if err != nil {
				return err
			}

			// if node is missing inventory allocation to cluster
			// then skip the HW generation, as this node doesnt yet have any addresses
			// allocated
			if !util.ConditionExists(inventory, seederv1alpha1.InventoryAllocatedToCluster) {
				r.Info("skipping node from hardware generation as it has not yet been processed for allocation to cluster", inventory.Name, inventory.Namespace)
				continue
			}

			workflow := tink.GenerateWorkflow(inventory, c)
			if err != nil {
				return err
			}
			// create hardware object
			err = controllerutil.SetOwnerReference(c, workflow, r.Scheme)
			if err != nil {
				return err
			}

			err = r.createOrUpdateWorkflow(ctx, workflow, inventory)
			if err != nil {
				return err
			}
		}

	}

	// check all inventory objects have correct conditions before updating cluster object
	conditionsPresent := true
	for _, v := range c.Spec.Nodes {
		iObj := &seederv1alpha1.Inventory{}
		if err := r.Get(ctx, types.NamespacedName{Name: v.InventoryReference.Name, Namespace: v.InventoryReference.Namespace}, iObj); err != nil {
			return err
		}
		if !util.ConditionExists(iObj, seederv1alpha1.TinkHardwareCreated) || !util.ConditionExists(iObj, seederv1alpha1.TinkTemplateCreated) || !util.ConditionExists(iObj, seederv1alpha1.TinkWorkflowCreated) {
			conditionsPresent = conditionsPresent && false
		}
	}

	if c.Status.Status == seederv1alpha1.ClusterNodesPatched {
		c.Status.Status = seederv1alpha1.ClusterTinkHardwareSubmitted
		return r.Status().Update(ctx, c)
	}

	return nil
}

func (r *ClusterTinkerbellWorkflowReconciler) createOrUpdateWorkflow(ctx context.Context, wf *tinkv1alpha1.Workflow, inventory *seederv1alpha1.Inventory) error {
	wfObj := &tinkv1alpha1.Workflow{}
	err := r.Get(ctx, types.NamespacedName{Name: wf.Name, Namespace: wf.Namespace}, wfObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if createErr := r.Create(ctx, wf); createErr != nil {
				return createErr
			}
		}
		return err
	}

	if !reflect.DeepEqual(wfObj.Spec, wf.Spec) {
		wfObj.Spec = wf.Spec
		if err := r.Update(ctx, wfObj); err != nil {
			return err
		}
	}

	return createOrUpdateInventoryConditions(ctx, inventory, seederv1alpha1.TinkWorkflowCreated, "tink workflow created", r.Client)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterTinkerbellWorkflowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&seederv1alpha1.Cluster{}).
		Watches(&tinkv1alpha1.Workflow{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, a client.Object) []reconcile.Request {
			var reconRequest []reconcile.Request
			owners := a.GetOwnerReferences()
			for _, o := range owners {
				if o.Kind == "Cluster" && o.APIVersion == "metal.harvesterhci.io/v1alpha1" {
					reconRequest = append(reconRequest, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: a.GetNamespace(),
							Name:      o.Name,
						},
					})
				}
			}
			return reconRequest
		})).
		Complete(r)
}
