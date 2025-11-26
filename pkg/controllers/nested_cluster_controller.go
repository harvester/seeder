package controllers

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/util"
)

type NestedClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logr.Logger
}

// reconcile NestedCluster objects
func (r *NestedClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	n := &seederv1alpha1.NestedCluster{}
	err := r.Get(ctx, req.NamespacedName, n)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.Error(err, "unable to fetch nestedcluster object")
		return ctrl.Result{}, err
	}

	if n.DeletionTimestamp.IsZero() {
		// ensure a finalizer exists
		if !controllerutil.ContainsFinalizer(n, seederv1alpha1.NestedClusterFinalizer) {
			controllerutil.AddFinalizer(n, seederv1alpha1.NestedClusterFinalizer)
			return ctrl.Result{}, r.Update(ctx, n)
		}
		// reconcile object and ensure associated VirtualMachinePool exists
	} else {
		return ctrl.Result{}, r.CleanupNestedCluster(ctx, n)
	}

	return ctrl.Result{}, r.EnsureClusterExists(ctx, n)
}

// CleanupNestedCluster handles cleanup of resources associated with the NestedCluster
// it will delete the cluster object and wait for it to be GC'd and then remove the finalizer
func (r *NestedClusterReconciler) CleanupNestedCluster(ctx context.Context, n *seederv1alpha1.NestedCluster) error {

	clusterObj := &seederv1alpha1.Cluster{}
	err := r.Get(ctx, client.ObjectKey{Namespace: n.Namespace, Name: n.Name}, clusterObj)

	if err != nil {
		// cluster object not found, proceed to remove finalizer
		// and ensure that the nestedcluster object can be deleted
		if apierrors.IsNotFound(err) {
			// Remove finalizer after cleanup
			if controllerutil.RemoveFinalizer(n, seederv1alpha1.NestedClusterFinalizer) {
				return r.Update(ctx, n)
			}
		}
		return fmt.Errorf("error fetching cluster object during deletion %s/%s: %w", n.Namespace, n.Name, err)
	}

	if clusterObj.DeletionTimestamp.IsZero() {
		// delete the cluster object
		if err := r.Delete(ctx, clusterObj); err != nil {
			return fmt.Errorf("error deleting cluster object %s/%s: %w", clusterObj.Namespace, clusterObj.Name, err)
		}
	}

	// default is to error since clusterObj is found
	// we return error to ensure object is requeued and we wait until cluster is completed deleted
	// this can take a while since seeder will attempt to power off all inventory objects before
	// allowing cleanup of underlying cluster object
	return fmt.Errorf("waiting for cluster object %s/%s to be deleted before removing finalizer", clusterObj.Namespace, clusterObj.Name)
}

// EnsureClusterExists will ensure that the underlying Cluster object exists
// and it will reconcile the status of cluster object into nested cluster
func (r *NestedClusterReconciler) EnsureClusterExists(ctx context.Context, n *seederv1alpha1.NestedCluster) error {
	// ensure inventory templates exist
	if err := r.ensureInventoryTemplatesExist(ctx, n); err != nil {
		return fmt.Errorf("error ensuring inventory templates for nested cluster %s/%s: %w", n.Namespace, n.Name, err)
	}

	// ensure inventoryTemplates are ready for use
	if err := r.ensureInventoryTemplatesAreReady(ctx, n); err != nil {
		return fmt.Errorf("error ensuring inventory templates are ready for nested cluster %s/%s: %w", n.Namespace, n.Name, err)
	}

	// ensure cluster object using inventory exists
	clusterObj := util.GenerateClusterFromNestedCluster(n)
	if err := reconcileObject(ctx, r.Client, clusterObj); err != nil {
		return fmt.Errorf("error reconciling cluster object for nested cluster %s/%s: %w", n.Namespace, n.Name, err)
	}

	// reconcile status from cluster into nestedcluster status
	existingCluster := &seederv1alpha1.Cluster{}
	err := r.Get(ctx, client.ObjectKey{Namespace: n.Namespace, Name: n.Name}, existingCluster)
	if err != nil {
		return fmt.Errorf("error fetching cluster object for nested cluster %s/%s: %w", n.Namespace, n.Name, err)
	}

	nCopy := n.DeepCopy()
	n.Status = existingCluster.Status
	if !reflect.DeepEqual(n.Status, nCopy.Status) {
		if err := r.Status().Update(ctx, n); err != nil {
			return fmt.Errorf("error updating status for nested cluster %s/%s: %w", n.Namespace, n.Name, err)
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NestedClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&seederv1alpha1.NestedCluster{}).
		Watches(&seederv1alpha1.Cluster{}, &handler.EnqueueRequestForObject{}). //cluster status updates are not being reconcilled
		// need to check and ensure they are applied to the owning nested cluster object
		Named("nestedclusters").
		Complete(r)
}

func (r *NestedClusterReconciler) ensureInventoryTemplatesExist(ctx context.Context, n *seederv1alpha1.NestedCluster) error {
	// generate inventoryTemplates based on current nestedcluster spec
	desiredTemplates := util.GenerateInventoryTemplates(n)

	inventoryTemplateList := &seederv1alpha1.InventoryTemplateList{}
	err := r.List(ctx, inventoryTemplateList, client.InNamespace(n.Namespace), client.MatchingLabels{seederv1alpha1.NestedClusterUIDLabelKey: string(n.GetUID())})
	if err != nil {
		return fmt.Errorf("error listing inventory templates for nested cluster %s/%s: %w", n.Namespace, n.Name, err)
	}

	toDeleteTemplates := map[string]seederv1alpha1.InventoryTemplate{}
	for _, it := range inventoryTemplateList.Items {
		toDeleteTemplates[it.Name] = it
	}

	// iterate over existing templates query from apiserver
	// and identify the ones which may need to be deleted
	// since they no longer match the configuration
	for _, desiredIT := range desiredTemplates {
		delete(toDeleteTemplates, desiredIT.Name)
	}

	for _, itToDelete := range toDeleteTemplates {
		if err := r.Delete(ctx, &itToDelete); err != nil {
			return fmt.Errorf("error deleting inventory template %s/%s: %w", itToDelete.Namespace, itToDelete.Name, err)
		}
	}

	// apply desired templates
	for _, obj := range desiredTemplates {
		err := reconcileObject(ctx, r.Client, obj)
		if err != nil {
			return fmt.Errorf("error reconciling inventorytemplates %s for nestedcluster %s: %w", obj.Name, n.Name, err)
		}
	}

	return nil
}

func (r *NestedClusterReconciler) ensureInventoryTemplatesAreReady(ctx context.Context, n *seederv1alpha1.NestedCluster) error {
	// fetch inventory templates associated with nested cluster
	inventoryTemplateList := &seederv1alpha1.InventoryTemplateList{}
	err := r.List(ctx, inventoryTemplateList, client.InNamespace(n.Namespace), client.MatchingLabels{seederv1alpha1.NestedClusterUIDLabelKey: string(n.GetUID())})
	if err != nil {
		return fmt.Errorf("error listing inventory templates for nested cluster %s/%s: %w", n.Namespace, n.Name, err)
	}

	for _, it := range inventoryTemplateList.Items {
		if it.Status.Status != seederv1alpha1.InventoryTemplateProvisioned {
			return fmt.Errorf("inventory template %s/%s not yet provisioned", it.Namespace, it.Name)
		}
	}

	return nil
}
