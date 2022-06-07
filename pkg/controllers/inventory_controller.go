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
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/harvester/bmaas/pkg/util"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		r.triggerReboot,
		r.reconcileBMCJob,
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
		for _, reconciler := range deletionReconcileList {
			if err := reconciler(ctx, inventoryObj); err != nil {
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
	if util.ConditionExists(i.Status.Conditions, bmaasv1alpha1.BMCObjectCreated) {
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

	id, err := uuid.NewUUID()
	if err != nil {
		return err
	}
	i.Status.HardwareID = id.String()
	i.Status.Conditions = util.CreateOrUpdateCondition(i.Status.Conditions, bmaasv1alpha1.BMCObjectCreated, "bmc object created")
	return r.Client.Status().Update(ctx, i)
}

// checkAndMarkNodeReady will check the power status of the BaseboardManagement Object and Mark the node ready
func (r *InventoryReconciler) checkAndMarkNodeReady(ctx context.Context, i *bmaasv1alpha1.Inventory) error {
	if util.ConditionExists(i.Status.Conditions, bmaasv1alpha1.BMCObjectCreated) {
		if i.Status.Status == bmaasv1alpha1.InventoryReady {
			return nil
		}

		// check status of boseboard object
		b := &rufio.BaseboardManagement{}
		err := r.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, b)
		if err != nil {
			r.Error(err, "error fetching associated baseboard object in checkAndMarkNodeReady")
			return err
		}

		// check if condition bmcv1alpha1.Contactable exists and is bmcv1alpha1.ConditionTrue
		if util.IsBaseboardReady(b) {
			i.Status.Status = bmaasv1alpha1.InventoryReady
			err = r.Status().Update(ctx, i)
			if err != nil {
				return err
			}
			// apply finalizer on inventory
			if !controllerutil.ContainsFinalizer(i, bmaasv1alpha1.InventoryFinalizer) {
				controllerutil.AddFinalizer(i, bmaasv1alpha1.InventoryFinalizer)
				return r.Update(ctx, i)
			}
		}
	}
	return nil
}

// handleBMCDeletion will reconcile deletion of BaseboardManagement objects {
func (r *InventoryReconciler) handleBaseboardDeletion(ctx context.Context, i *bmaasv1alpha1.Inventory) error {
	// if no status is present then nothing is needed yet as BMC has not yet been created
	if i.Status.Status == bmaasv1alpha1.InventoryReady {
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
			// reset status to re-trigger recreation of baseboard objects
			i.Status.Status = ""
			util.RemoveCondition(i.Status.Conditions, bmaasv1alpha1.BMCObjectCreated)
			return r.Status().Update(ctx, i)
		}
	}

	return nil
}

// handleInventoryDeletion cleans up the finalizer on boseboard object allowing it to be cleaned up
func (r *InventoryReconciler) handleInventoryDeletion(ctx context.Context, i *bmaasv1alpha1.Inventory) error {
	if controllerutil.ContainsFinalizer(i, bmaasv1alpha1.InventoryFinalizer) {
		b := &rufio.BaseboardManagement{}
		var skipcleanup bool
		err := r.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, b)
		if err != nil {
			if apierrors.IsNotFound(err) {
				skipcleanup = true
			} else {
				return err
			}
		}
		if !skipcleanup {
			if controllerutil.ContainsFinalizer(b, bmaasv1alpha1.InventoryFinalizer) {
				controllerutil.RemoveFinalizer(b, bmaasv1alpha1.InventoryFinalizer)
			}
			if err := r.Update(ctx, b); err != nil {
				return err
			}
		}

		if controllerutil.ContainsFinalizer(i, bmaasv1alpha1.InventoryFinalizer) {
			controllerutil.RemoveFinalizer(i, bmaasv1alpha1.InventoryFinalizer)
			return r.Update(ctx, i)
		}
	}

	return nil
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

// triggerReboot will reboot the machine using the BMCJob object
func (r *InventoryReconciler) triggerReboot(ctx context.Context, i *bmaasv1alpha1.Inventory) error {
	// if tink hardware has been created and inventory is allocated to a cluster
	// then reboot the hardware using BMC tasks
	if i.Status.Status == bmaasv1alpha1.InventoryReady && util.ConditionExists(i.Status.Conditions, bmaasv1alpha1.TinkWorkflowCreated) && util.ConditionExists(i.Status.Conditions, bmaasv1alpha1.InventoryAllocatedToCluster) && !util.ConditionExists(i.Status.Conditions, bmaasv1alpha1.BMCJobSubmitted) {
		// submit BMC task
		off := rufio.HardPowerOff
		on := rufio.PowerOn
		job := &rufio.BMCJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-reboot", i.Name),
				Namespace: i.Namespace,
			},
			Spec: rufio.BMCJobSpec{
				BaseboardManagementRef: rufio.BaseboardManagementRef{
					Name:      i.Name,
					Namespace: i.Namespace,
				},
				Tasks: []rufio.Task{
					{
						PowerAction: &off,
					},
					{
						OneTimeBootDeviceAction: &rufio.OneTimeBootDeviceAction{
							Devices: []rufio.BootDevice{
								rufio.PXE,
							},
							EFIBoot: false,
						},
					},
					{
						PowerAction: &on,
					},
				},
			},
		}
		err := controllerutil.SetOwnerReference(i, job, r.Scheme)
		if err != nil {
			return err
		}
		err = r.Create(ctx, job)
		if err != nil {
			return err
		}

		i.Status.Conditions = util.CreateOrUpdateCondition(i.Status.Conditions, bmaasv1alpha1.BMCJobSubmitted, "BMCJob submitted")

		return r.Status().Update(ctx, i)
	}

	return nil
}

// reconcileBMCJob will update the BMCJob conditions to reflect current state of the job for specific inventory
func (r *InventoryReconciler) reconcileBMCJob(ctx context.Context, i *bmaasv1alpha1.Inventory) error {

	if util.ConditionExists(i.Status.Conditions, bmaasv1alpha1.BMCJobSubmitted) {
		j := &rufio.BMCJob{}
		err := r.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: fmt.Sprintf("%s-reboot", i.Name)}, j)
		if err != nil {
			return err
		}

		if j.HasCondition(rufio.JobCompleted, rufio.ConditionTrue) {
			i.Status.Conditions = util.CreateOrUpdateCondition(i.Status.Conditions, bmaasv1alpha1.BMCJobComplete, "")
		}

		if j.HasCondition(rufio.JobFailed, rufio.ConditionTrue) {
			i.Status.Conditions = util.CreateOrUpdateCondition(i.Status.Conditions, bmaasv1alpha1.BMCJobError, "")
		}
		util.RemoveCondition(i.Status.Conditions, bmaasv1alpha1.BMCJobSubmitted)
		return r.Status().Update(ctx, i)
	}
	return nil
}
