/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/harvester/bmaas/pkg/util"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	bmaasv1alpha1 "github.com/harvester/bmaas/pkg/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InventoryReconciler reconciles a Inventory object
type InventoryReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logr.Logger
}

type inventoryReconciler func(context.Context, *bmaasv1alpha1.Inventory) error

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
func (r *InventoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Info("Reconcilling inventory objects", req.Name, req.Namespace)
	inventoryObj := &bmaasv1alpha1.Inventory{}

	err := r.Get(ctx, req.NamespacedName, inventoryObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.Error(err, "Failed to get Inventory Object")
		return ctrl.Result{}, err
	}

	reconcileList := []inventoryReconciler{
		r.manageBaseboardObject,
		r.checkAndMarkNodeReady,
		r.handleBaseboardDeletion,
	}

	deletionReconcileList := []inventoryReconciler{
		r.handleInventoryDeletion,
	}
	if inventoryObj.DeletionTimestamp.IsZero() {
		for _, reconciler := range reconcileList {
			if err := reconciler(ctx, inventoryObj); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		for _, reconcile := range deletionReconcileList {
			if err := reconcile(ctx, inventoryObj); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// manageBMCObject checks if an associated BaseboardManagement Object exists else creates one
// and sets the appropriate ownership
func (r *InventoryReconciler) manageBaseboardObject(ctx context.Context, i *bmaasv1alpha1.Inventory) error {
	// already in desired state. No further action needed
	if i.Status.Status == bmaasv1alpha1.BMCObjectCreated {
		return nil
	}

	err := util.CheckSecretExists(ctx, r.Client, r.Logger, i.Spec.BaseboardManagementSpec.Connection.AuthSecretRef)
	if err != nil {
		return err
	}

	err = util.CheckAndCreateBaseBoardObject(ctx, r.Client, r.Logger, i, r.Scheme)
	if err != nil {
		return err
	}

	i.Status.Status = bmaasv1alpha1.BMCObjectCreated
	return r.Client.Status().Update(ctx, i)
}

// checkAndMarkNodeReady will check the power status of the BaseboardManagement Object and Mark the node ready
func (r *InventoryReconciler) checkAndMarkNodeReady(ctx context.Context, i *bmaasv1alpha1.Inventory) error {
	if i.Status.Status == bmaasv1alpha1.InventoryReady {

		return nil
	}
	b := &rufio.BaseboardManagement{}
	err := r.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, b)
	if err != nil {
		r.Error(err, "error fetching associated baseboard object in checkAndMarkNodeReady")
		return err
	}

	if b.Spec.Power == b.Status.Power {
		i.Status.Status = bmaasv1alpha1.InventoryReady
		return r.Client.Status().Update(ctx, i)

	}

	return nil
}

// handleBMCDeletion will reconcile deletion of BaseboardManagement objects {
func (r *InventoryReconciler) handleBaseboardDeletion(ctx context.Context, i *bmaasv1alpha1.Inventory) error {
	// if no status is present then nothing is needed yet as BMC has not yet been created
	if i.Status.Status != "" {
		b := &rufio.BaseboardManagement{}
		err := r.Get(ctx, types.NamespacedName{Name: i.Name, Namespace: i.Namespace}, b)
		if err != nil {
			r.Error(err, "error looking up baseboard object")
			return err
		}
		if !b.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(b, bmaasv1alpha1.InventoryFinalizer) {
			controllerutil.RemoveFinalizer(b, bmaasv1alpha1.InventoryFinalizer)
			err = r.Update(ctx, b)
			if err != nil {
				r.Error(err, "error removing finalizer from baseboard object")
				return err
			}
		}
		// reset status to re-trigger recreation of baseboard objects
		i.Status.Status = ""
		return r.Status().Update(ctx, i)
	}

	return nil
}

// handleInventoryDeletion cleans up the finalizer on boseboard object allowing it to be cleaned up
func (r *InventoryReconciler) handleInventoryDeletion(ctx context.Context, i *bmaasv1alpha1.Inventory) error {
	b := &rufio.BaseboardManagement{}
	err := r.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, b)
	if err != nil {
		return err
	}
	if controllerutil.ContainsFinalizer(b, bmaasv1alpha1.InventoryFinalizer) {
		controllerutil.RemoveFinalizer(b, bmaasv1alpha1.InventoryFinalizer)
	}
	return r.Update(ctx, b)
}

// SetupWithManager sets up the controller with the Manager.
func (r *InventoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bmaasv1alpha1.Inventory{}).
		Watches(&source.Kind{Type: &rufio.BaseboardManagement{}}, handler.EnqueueRequestsFromMapFunc(func(a client.Object) []reconcile.Request {
			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{
					Namespace: a.GetNamespace(),
					Name:      a.GetName(),
				},
			},
			}
		})).
		Complete(r)
}
