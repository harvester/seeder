package controllers

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/util"
)

// InventoryReconciler reconciles a Inventory object
type AddressPoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logr.Logger
}

type addressPoolReconciler func(context.Context, *seederv1alpha1.AddressPool) error

//+kubebuilder:rbac:groups=metal.harvesterhci.io,resources=inventories,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.harvesterhci.io,resources=inventories/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.harvesterhci.io,resources=inventories/finalizers,verbs=update
//+kubebuilder:rbac:groups=metal.harvesterhci.io,resources=addresspools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.harvesterhci.io,resources=addresspools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.harvesterhci.io,resources=addresspools/finalizers,verbs=update
//+kubebuilder:rbac:groups=tinkerbell.org,resources=hardware,verbs=get;list;watch;create;update;patch;delete

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
	pool := &seederv1alpha1.AddressPool{}

	err := r.Get(ctx, req.NamespacedName, pool)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.Error(err, "Failed to get AddressPool Object")
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
func (r *AddressPoolReconciler) reconcilePoolCapacity(ctx context.Context, poolObj *seederv1alpha1.AddressPool) error {

	pool := poolObj.DeepCopy()
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

		if !controllerutil.ContainsFinalizer(pool, seederv1alpha1.AddressPoolFinalizer) {
			controllerutil.AddFinalizer(pool, seederv1alpha1.AddressPoolFinalizer)
			return r.Client.Update(ctx, pool)
		}
	}

	// reconcile capacity and update status for pool

	if pool.Status.Status == seederv1alpha1.PoolReady && len(pool.Status.AddressAllocation) == pool.Status.AvailableAddresses {
		pool.Status.Status = seederv1alpha1.PoolExhausted
		return r.Client.Status().Update(ctx, pool)
	}

	if pool.Status.Status == seederv1alpha1.PoolExhausted && len(pool.Status.AddressAllocation) < pool.Status.AvailableAddresses {
		pool.Status.Status = seederv1alpha1.PoolReady
		return r.Client.Status().Update(ctx, pool)
	}

	return nil
}

// deleteAddressPool will ensure that none of the IP's is in use before removing finalizer
func (r *AddressPoolReconciler) deleteAddressPool(ctx context.Context, poolObj *seederv1alpha1.AddressPool) error {
	pool := poolObj.DeepCopy()
	if !pool.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(pool, seederv1alpha1.AddressPoolFinalizer) {
		var addressInUse bool
		var err error
		for address, ref := range pool.Status.AddressAllocation {
			if reflect.DeepEqual(ref, seederv1alpha1.ObjectReferenceWithKind{}) {
				delete(pool.Status.AddressAllocation, address)
				continue
			}
			if ref.Kind == seederv1alpha1.KindCluster {
				addressInUse, err = r.lookupClusterVIP(ctx, ref.ObjectReference, address)
			} else {
				addressInUse, err = r.lookupInventoryAddress(ctx, ref.ObjectReference, address)
			}
		}

		if err != nil {
			return err
		}

		if addressInUse {
			return fmt.Errorf("one of the address in addresspool %s is still in use, requeuing", pool.Name)
		}
		controllerutil.RemoveFinalizer(pool, seederv1alpha1.AddressPoolFinalizer)
		return r.Client.Update(ctx, pool)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AddressPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&seederv1alpha1.AddressPool{}).
		Complete(r)
}

func (r *AddressPoolReconciler) lookupClusterVIP(ctx context.Context, obj seederv1alpha1.ObjectReference, address string) (bool, error) {
	c := &seederv1alpha1.Cluster{}
	err := r.Get(ctx, types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}, c)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	if c.Status.ClusterAddress == address {
		return true, nil
	}

	return false, nil
}

func (r *AddressPoolReconciler) lookupInventoryAddress(ctx context.Context, obj seederv1alpha1.ObjectReference, address string) (bool, error) {
	c := &seederv1alpha1.Inventory{}
	err := r.Get(ctx, types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}, c)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	if c.Status.PXEBootInterface.Address == address {
		return true, nil
	}

	return false, nil
}
