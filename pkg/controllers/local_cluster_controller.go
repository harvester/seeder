package controllers

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/util"
)

type localClusterReconciler func(context.Context, *seederv1alpha1.Inventory) error

type LocalClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logr.Logger
	record.EventRecorder
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Cluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *LocalClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Info("Reconcilling inventory for local cluster objects", req.Name, req.Namespace)

	i := &seederv1alpha1.Inventory{}

	err := r.Get(ctx, req.NamespacedName, i)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.Error(err, "unable to fetch inventory object")
		return ctrl.Result{}, err
	}

	if val, ok := i.Annotations[seederv1alpha1.LocalInventoryAnnotation]; !ok || val != "true" {
		r.Info("skipping inventory object as it doesn't seem to be related to local inventory", req.Name, req.Namespace)
		return ctrl.Result{}, nil
	}

	reconcileList := []localClusterReconciler{
		r.addToLocalCluster,
		r.ensureMachineExists,
		r.manageStatus,
	}

	deletionReconcileList := []inventoryReconciler{
		r.removeFromLocalCluster,
	}

	if i.DeletionTimestamp.IsZero() {
		for _, reconciler := range reconcileList {
			if err := reconciler(ctx, i); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		for _, reconciler := range deletionReconcileList {
			if err := reconciler(ctx, i); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *LocalClusterReconciler) addToLocalCluster(ctx context.Context, i *seederv1alpha1.Inventory) error {
	localCluster := &seederv1alpha1.Cluster{}
	err := r.Get(ctx, types.NamespacedName{Name: seederv1alpha1.DefaultLocalClusterName, Namespace: seederv1alpha1.DefaultLocalClusterNamespace}, localCluster)
	if err != nil {
		return fmt.Errorf("error fetching local cluster: %v", err)
	}

	var skipUpdate bool
	for _, v := range localCluster.Spec.Nodes {
		if v.InventoryReference.Name == i.Name && v.InventoryReference.Namespace == i.Namespace {
			// inventory already exists. no further action needed
			skipUpdate = true
		}
	}

	if !skipUpdate {
		localCluster.Spec.Nodes = append(localCluster.Spec.Nodes, seederv1alpha1.NodeConfig{
			InventoryReference: seederv1alpha1.ObjectReference{
				Name:      i.Name,
				Namespace: i.Namespace,
			},
		})

		err = r.Update(ctx, localCluster)
		if err != nil {
			return fmt.Errorf("error updating local cluster")
		}

		r.EventRecorder.Event(localCluster, "Normal", "ClusterUpdated", fmt.Sprintf("inventory %s in namespace %s added to local cluster", i.Name, i.Namespace))
	}

	// apply finalizer
	if !controllerutil.ContainsFinalizer(i, seederv1alpha1.InventoryFinalizer) {
		controllerutil.AddFinalizer(i, seederv1alpha1.InventoryFinalizer)
		return r.Update(ctx, i)
	}

	return nil
}

func (r *LocalClusterReconciler) removeFromLocalCluster(ctx context.Context, i *seederv1alpha1.Inventory) error {
	localCluster := &seederv1alpha1.Cluster{}
	err := r.Get(ctx, types.NamespacedName{Name: seederv1alpha1.DefaultLocalClusterName, Namespace: seederv1alpha1.DefaultLocalClusterNamespace}, localCluster)
	if err != nil {
		return fmt.Errorf("error fetching local cluster: %v", err)
	}

	var updateCluster bool
	for idx, v := range localCluster.Spec.Nodes {
		if v.InventoryReference.Name == i.Name && v.InventoryReference.Namespace == i.Namespace {
			// inventory exists, and needs to be removed
			updateCluster = true
			localCluster.Spec.Nodes = append(localCluster.Spec.Nodes[:idx], localCluster.Spec.Nodes[idx+1:]...)
		}
	}

	if updateCluster {
		err = r.Update(ctx, localCluster)
		if err != nil {
			return fmt.Errorf("error updating local cluster")
		}
		r.EventRecorder.Event(localCluster, "Normal", "ClusterUpdated", fmt.Sprintf("inventory %s in namespace %s removed from local cluster", i.Name, i.Namespace))
	}

	// remove finalizer
	if controllerutil.ContainsFinalizer(i, seederv1alpha1.InventoryFinalizer) {
		controllerutil.RemoveFinalizer(i, seederv1alpha1.InventoryFinalizer)
		return r.Update(ctx, i)
	}

	return nil
}

// TODO: Remove as likely to be handled via node ownership
func (r *LocalClusterReconciler) reconcileNodes(ctx context.Context, i *seederv1alpha1.Inventory) error {
	node := &corev1.Node{}
	err := r.Get(ctx, types.NamespacedName{Name: i.Name, Namespace: ""}, node)
	var deleteInventory bool
	if err != nil {
		if apierrors.IsNotFound(err) {
			deleteInventory = true
		} else {
			return err
		}
	}

	if deleteInventory || node.DeletionTimestamp != nil {
		// trigger delete of associated inventory object
		return r.Delete(ctx, i)
	}

	return nil
}

func (r *LocalClusterReconciler) ensureMachineExists(ctx context.Context, i *seederv1alpha1.Inventory) error {
	machine := &rufio.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      i.Name,
			Namespace: i.Namespace,
		},
		Spec: i.Spec.BaseboardManagementSpec,
	}

	err := controllerutil.SetOwnerReference(i, machine, r.Scheme)
	if err != nil {
		return fmt.Errorf("error setting owner references for machine %s: %v", machine.Name, err)
	}

	err = controllerutil.SetControllerReference(i, machine, r.Scheme)
	if err != nil {
		return fmt.Errorf("error setting controller reference for machine %s: %v", machine.Name, err)
	}

	// check if machine exists
	machineObj := &rufio.Machine{}
	err = r.Get(ctx, types.NamespacedName{Name: machine.Name, Namespace: machine.Namespace}, machineObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, machine)
		}
	}
	return err
}

// manageStatus simply reads the LocalInventoryStatusAnnotation value and tries to apply it status for inventory status
// subresource
func (r *LocalClusterReconciler) manageStatus(ctx context.Context, iObj *seederv1alpha1.Inventory) error {
	i := iObj.DeepCopy()
	nodeName, ok := i.Annotations[seederv1alpha1.LocalInventoryNodeName]
	if !ok {
		return fmt.Errorf("missing annotation %s on inventory %s", seederv1alpha1.LocalInventoryNodeName, i.Name)
	}

	if i.Status.Status == seederv1alpha1.InventoryReady && util.ConditionExists(i, seederv1alpha1.InventoryAllocatedToCluster) {
		return nil
	}

	nodeObj := &corev1.Node{}

	err := r.Get(ctx, types.NamespacedName{Name: nodeName}, nodeObj)
	if err != nil {
		return fmt.Errorf("error querying node %s: %v", nodeName, err)
	}


	for _, v := range nodeObj.Status.Addresses {
		if v.Type == corev1.NodeInternalIP {
			i.Status.PXEBootInterface.Address = v.Address
		}
	}

	if i.Status.Status == seederv1alpha1.InventoryReady && !util.ConditionExists(i, seederv1alpha1.InventoryAllocatedToCluster) {
		util.CreateOrUpdateCondition(i, seederv1alpha1.InventoryAllocatedToCluster, "node assigned to local cluster")
	}

	if !reflect.DeepEqual(iObj, i) {
		return r.Status().Update(ctx, i)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LocalClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&seederv1alpha1.Inventory{}).
		Owns(&rufio.Machine{}).
		Complete(r)
}
