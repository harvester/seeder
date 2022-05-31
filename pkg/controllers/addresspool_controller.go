package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	bmaasv1alpha1 "github.com/harvester/bmaas/pkg/api/v1alpha1"
	"github.com/harvester/bmaas/pkg/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// InventoryReconciler reconciles a Inventory object
type AddressPoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logr.Logger
}

type addressPoolReconciler func(context.Context, *bmaasv1alpha1.AddressPool) error

//+kubebuilder:rbac:groups=metal.harvesterhci.io,resources=inventories,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.harvesterhci.io,resources=inventories/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.harvesterhci.io,resources=inventories/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Inventory object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *AddressPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Info("Reconcilling addresspool objects", req.Name, req.Namespace)
	pool := &bmaasv1alpha1.AddressPool{}

	err := r.Get(ctx, req.NamespacedName, pool)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.Error(err, "Failed to get Inventory Object")
		return ctrl.Result{}, err
	}

	reconcileList := []addressPoolReconciler{
		r.reconcilePoolCapacity,
	}

	deletionReconcileList := []addressPoolReconciler{
		r.deleteAddressPool,
	}

	if pool.DeletionTimestamp.IsZero() {
		for _, reconciler := range reconcileList {
			if err := reconciler(ctx, pool); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		for _, reconciler := range deletionReconcileList {
			if err := reconciler(ctx, pool); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// reconcilePoolCapacity will mark the pool as exhausted if all addresses are used up, and make it ready once addresses are freed up again
func (r *AddressPoolReconciler) reconcilePoolCapacity(ctx context.Context, pool *bmaasv1alpha1.AddressPool) error {

	// initial reconcile
	if pool.Status.Status == "" {
		status, err := util.GenerateAddressPoolStatus(pool)
		if err != nil {
			return err
		}
		pool.Status = *status
		err = r.Client.Status().Update(ctx, pool)
		if err != nil {
			return err
		}

		if !controllerutil.ContainsFinalizer(pool, bmaasv1alpha1.AddressPoolFinalizer) {
			controllerutil.AddFinalizer(pool, bmaasv1alpha1.AddressPoolFinalizer)
			return r.Client.Update(ctx, pool)
		}
	}

	// reconcile capacity and update status for pool

	if pool.Status.Status == bmaasv1alpha1.PoolReady && len(pool.Status.AddressAllocation) == pool.Status.AvailableAddresses {
		pool.Status.Status = bmaasv1alpha1.PoolExhausted
		return r.Client.Status().Update(ctx, pool)
	}

	if pool.Status.Status == bmaasv1alpha1.PoolExhausted && len(pool.Status.AddressAllocation) < pool.Status.AvailableAddresses {
		pool.Status.Status = bmaasv1alpha1.PoolReady
		return r.Client.Status().Update(ctx, pool)
	}

	return nil
}

// deleteAddressPool will ensure that none of the IP's is in use before removing finalizer
func (r *AddressPoolReconciler) deleteAddressPool(ctx context.Context, pool *bmaasv1alpha1.AddressPool) error {
	if !pool.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(pool, bmaasv1alpha1.AddressPoolFinalizer) {
		var addressInUse bool
		for address, ref := range pool.Status.AddressAllocation {
			i := &bmaasv1alpha1.Inventory{}
			err := r.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, i)
			if err != nil {
				return err
			}

			if address == i.Status.PXEBootInterface.Address {
				addressInUse = true
			}
		}
		if addressInUse {
			return fmt.Errorf("one of the address in addresspool %s is still in use, requeuing", pool.Name)
		}
		controllerutil.RemoveFinalizer(pool, bmaasv1alpha1.AddressPoolFinalizer)
		return r.Client.Update(ctx, pool)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AddressPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bmaasv1alpha1.AddressPool{}).
		Complete(r)
}
