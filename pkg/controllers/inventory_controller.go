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
	"reflect"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/util"
)

// InventoryReconciler reconciles a Inventory object
type InventoryReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logr.Logger
}

type inventoryReconciler func(context.Context, *seederv1alpha1.Inventory) error

//+kubebuilder:rbac:groups=metal.harvesterhci.io,resources=inventories,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.harvesterhci.io,resources=inventories/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.harvesterhci.io,resources=inventories/finalizers,verbs=update
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=bmcjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=bmcjobs/status,verbs=get
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=baseboardmanagements,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=baseboardmanagements/status,verbs=get
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

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
	inventoryObj := &seederv1alpha1.Inventory{}

	err := r.Get(ctx, req.NamespacedName, inventoryObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.Error(err, "Failed to get Inventory Object")
		return ctrl.Result{}, err
	}

	reconcileList := []inventoryReconciler{r.triggerPowerAction, r.manageBaseboardObject, r.checkAndMarkNodeReady,
		r.handleBaseboardDeletion, r.reconcileBMCJob, r.housekeepingBMCJob, r.hasMachineSpecChanged}
	// if inventory object has LocalInventoryAnnotation then skip reconcile as this will be handled by the local_cluster_controller
	if _, ok := inventoryObj.Annotations[seederv1alpha1.LocalInventoryAnnotation]; !ok {
		reconcileList = append(reconcileList, r.triggerReboot, r.inventoryFreed)
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
func (r *InventoryReconciler) manageBaseboardObject(ctx context.Context, iObj *seederv1alpha1.Inventory) error {
	i := iObj.DeepCopy()

	// inventory already in desired state. No further action needed
	if util.ConditionExists(i, seederv1alpha1.BMCObjectCreated) {
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
	util.CreateOrUpdateCondition(i, seederv1alpha1.BMCObjectCreated, "bmc object created")
	return r.Client.Status().Update(ctx, i)
}

// checkAndMarkNodeReady will check the power status of the BaseboardManagement Object and Mark the node ready
func (r *InventoryReconciler) checkAndMarkNodeReady(ctx context.Context, iObj *seederv1alpha1.Inventory) error {
	i := iObj.DeepCopy()
	if util.ConditionExists(i, seederv1alpha1.BMCObjectCreated) {
		if i.Status.Status == seederv1alpha1.InventoryReady {
			return nil
		}

		b, err := util.FetchAndUpdateBaseBoard(ctx, r.Client, r.Logger, i, r.Scheme)
		if err != nil {
			return fmt.Errorf("error during FetchAndUpdateBaseBoard: %v", err)
		}

		// check if condition bmcv1alpha1.Contactable exists and is bmcv1alpha1.ConditionTrue
		if util.IsBaseboardReady(b) {
			i.Status.Status = seederv1alpha1.InventoryReady
			if util.ConditionExists(i, seederv1alpha1.MachineNotContactable) {
				util.RemoveCondition(i, seederv1alpha1.MachineNotContactable)
			}
			err = r.Status().Update(ctx, i)
			if err != nil {
				return err
			}
			// apply finalizer on inventory
			if !controllerutil.ContainsFinalizer(i, seederv1alpha1.InventoryFinalizer) {
				controllerutil.AddFinalizer(i, seederv1alpha1.InventoryFinalizer)
				return r.Update(ctx, i)
			}
		}

		if ok, msg := util.IsMachineNotContactable(b); ok {
			if !util.ConditionExists(i, seederv1alpha1.MachineNotContactable) {
				util.SetErrorCondition(i, seederv1alpha1.MachineNotContactable, msg)
				return r.Status().Update(ctx, i)
			}
		}
	}
	return nil
}

// handleBMCDeletion will reconcile deletion of BaseboardManagement objects {
func (r *InventoryReconciler) handleBaseboardDeletion(ctx context.Context, iObj *seederv1alpha1.Inventory) error {
	i := iObj.DeepCopy()
	// if no status is present then nothing is needed yet as BMC has not yet been created
	if i.Status.Status == seederv1alpha1.InventoryReady {
		b := &rufio.Machine{}
		err := r.Get(ctx, types.NamespacedName{Name: i.Name, Namespace: i.Namespace}, b)
		if err != nil {
			r.Error(err, "error looking up baseboard object")
			return err
		}
		if !b.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(b, seederv1alpha1.InventoryFinalizer) {
			controllerutil.RemoveFinalizer(b, seederv1alpha1.InventoryFinalizer)
			err = r.Update(ctx, b)
			if err != nil {
				r.Error(err, "error removing finalizer from baseboard object")
				return err
			}
			// reset status to re-trigger recreation of baseboard objects
			i.Status.Status = ""
			util.RemoveCondition(i, seederv1alpha1.BMCObjectCreated)
			return r.Status().Update(ctx, i)
		}
	}

	return nil
}

// handleInventoryDeletion cleans up the finalizer on boseboard object allowing it to be cleaned up
func (r *InventoryReconciler) handleInventoryDeletion(ctx context.Context, iObj *seederv1alpha1.Inventory) error {
	i := iObj.DeepCopy()
	if controllerutil.ContainsFinalizer(i, seederv1alpha1.InventoryFinalizer) {
		b := &rufio.Machine{}
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
			if controllerutil.ContainsFinalizer(b, seederv1alpha1.InventoryFinalizer) {
				controllerutil.RemoveFinalizer(b, seederv1alpha1.InventoryFinalizer)
			}
			if err := r.Update(ctx, b); err != nil {
				return err
			}
		}

		if controllerutil.ContainsFinalizer(i, seederv1alpha1.InventoryFinalizer) {
			controllerutil.RemoveFinalizer(i, seederv1alpha1.InventoryFinalizer)
			return r.Update(ctx, i)
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *InventoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&seederv1alpha1.Inventory{}).
		Watches(&source.Kind{Type: &rufio.Machine{}}, handler.EnqueueRequestsFromMapFunc(func(a client.Object) []reconcile.Request {
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
func (r *InventoryReconciler) triggerReboot(ctx context.Context, iObj *seederv1alpha1.Inventory) error {
	// if tink hardware has been created and inventory is allocated to a cluster
	// then reboot the hardware using BMC tasks
	i := iObj.DeepCopy()
	if i.Status.Status == seederv1alpha1.InventoryReady && util.ConditionExists(i, seederv1alpha1.TinkWorkflowCreated) && util.ConditionExists(i, seederv1alpha1.InventoryAllocatedToCluster) && !util.ConditionExists(i, seederv1alpha1.BMCJobSubmitted) {
		// submit BMC task
		i.Spec.PowerActionRequested = seederv1alpha1.NodePowerActionReboot
		return r.Update(ctx, i)
	}

	return nil
}

// reconcileBMCJob will update the BMCJob conditions to reflect current state of the job for specific inventory
func (r *InventoryReconciler) reconcileBMCJob(ctx context.Context, iObj *seederv1alpha1.Inventory) error {
	i := iObj.DeepCopy()
	if util.ConditionExists(i, seederv1alpha1.BMCJobSubmitted) {
		j := &rufio.Job{}
		var completed bool
		err := r.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Status.PowerAction.LastJobName}, j)
		if err != nil {
			return err
		}

		if j.HasCondition(rufio.JobCompleted, rufio.ConditionTrue) {
			util.CreateOrUpdateCondition(i, seederv1alpha1.BMCJobComplete, "")
			i.Status.PowerAction.LastActionStatus = seederv1alpha1.NodeJobComplete
			completed = true
		}

		if j.HasCondition(rufio.JobFailed, rufio.ConditionTrue) {
			var message string
			for _, c := range j.Status.Conditions {
				if c.Type == rufio.JobFailed && c.Status == rufio.ConditionTrue {
					message = c.Message
				}
			}
			i.Status.PowerAction.LastActionStatus = seederv1alpha1.NodeJobFailed
			util.CreateOrUpdateCondition(i, seederv1alpha1.BMCJobError, message)
			completed = true
		}

		// job has completed, BMCJobSubmitted condition can be removed to avoid
		// further reconciles by reconcileBMCJob handler
		if completed {
			util.RemoveCondition(i, seederv1alpha1.BMCJobSubmitted)
			return r.Status().Update(ctx, i)
		}
		return fmt.Errorf("bmcjob %s not yet completed, requeuing", j.Name)
	}
	return nil
}

func (r *InventoryReconciler) inventoryFreed(ctx context.Context, iObj *seederv1alpha1.Inventory) error {
	i := iObj.DeepCopy()
	if util.ConditionExists(i, seederv1alpha1.InventoryFreed) {
		// check and submit a power off job
		var notFound bool

		j := util.GenerateJob(i.Name, i.Namespace, "shutdown")
		jobObj := &rufio.Job{}
		err := r.Get(ctx, types.NamespacedName{Namespace: j.Namespace, Name: j.Name}, jobObj)
		if err != nil {
			if apierrors.IsNotFound(err) {
				notFound = true
			} else {
				return err
			}
		}

		if notFound {
			if err := controllerutil.SetOwnerReference(i, j, r.Scheme); err != nil {
				return err
			}
			if err := r.Create(ctx, j); err != nil {
				return err
			}
		}

		// trigger status update
		util.RemoveCondition(i, seederv1alpha1.InventoryFreed)
		return r.Status().Update(ctx, i)
	}
	return nil
}

func (r *InventoryReconciler) housekeepingBMCJob(ctx context.Context, iObj *seederv1alpha1.Inventory) error {
	i := iObj.DeepCopy()
	if !util.ConditionExists(i, seederv1alpha1.InventoryAllocatedToCluster) && !util.ConditionExists(i, seederv1alpha1.InventoryFreed) {
		bmcjoblist := &rufio.JobList{}
		l, err := labels.Parse(fmt.Sprintf("inventory.metal.harvesterhci.io=%s", i.Name))
		if err != nil {
			return err
		}
		if err := r.List(ctx, bmcjoblist, &client.ListOptions{LabelSelector: l}); err != nil {
			return err
		}

		for _, v := range bmcjoblist.Items {
			var completed bool
			for _, c := range v.Status.Conditions {
				if c.Type == rufio.JobCompleted {
					completed = true
				}
			}
			if completed {
				if err := r.Delete(ctx, &v); err != nil {
					return err
				}
			}

		}

		util.RemoveCondition(i, seederv1alpha1.BMCJobSubmitted)
		util.RemoveCondition(i, seederv1alpha1.BMCJobComplete)
		return r.Status().Update(ctx, i)
	}

	return nil
}

func (r *InventoryReconciler) triggerPowerAction(ctx context.Context, iObj *seederv1alpha1.Inventory) error {
	i := iObj.DeepCopy()
	if i.Status.Status == seederv1alpha1.InventoryReady && !util.ConditionExists(i, seederv1alpha1.BMCJobSubmitted) && i.Spec.PowerActionRequested != "" && i.Status.PowerAction.LastJobName == "" {
		// if job name is not present then create one
		job := util.GenerateJob(i.Name, i.Namespace, i.Spec.PowerActionRequested)
		if job == nil {
			return fmt.Errorf("unsupported action, can not generate job for inventory %s", i.Name)
		}
		err := controllerutil.SetOwnerReference(i, job, r.Scheme)
		if err != nil {
			return fmt.Errorf("error setting owner reference on job %s: %v", job.Name, err)
		}
		r.Info("creating job", job.Name, job.Namespace)
		err = r.Create(ctx, job)
		if err != nil {
			return fmt.Errorf("error creating power action job: %v", err)
		}
		i.Status.PowerAction.LastActionStatus = ""
		i.Status.PowerAction.LastJobName = job.Name
		util.CreateOrUpdateCondition(i, seederv1alpha1.BMCJobSubmitted, "BMCJob Submitted")
		util.RemoveCondition(i, seederv1alpha1.BMCJobError)
		util.RemoveCondition(i, seederv1alpha1.BMCJobComplete)
	}

	if !reflect.DeepEqual(iObj.Status, i.Status) {
		return r.Status().Update(ctx, i)
	}
	return nil
}

func (r *InventoryReconciler) hasMachineSpecChanged(ctx context.Context, iObj *seederv1alpha1.Inventory) error {
	i := iObj.DeepCopy()
	existingObj := &rufio.Machine{}
	err := r.Get(ctx, types.NamespacedName{Name: i.Name, Namespace: i.Namespace}, existingObj)
	if err != nil {
		return err
	}

	if !reflect.DeepEqual(existingObj.Spec, i.Spec.BaseboardManagementSpec) && seederv1alpha1.BMCObjectCreated.IsTrue(i) {
		seederv1alpha1.BMCObjectCreated.False(i)
		i.Status.Status = ""
		return r.Status().Update(ctx, i)
	}

	return nil
}
