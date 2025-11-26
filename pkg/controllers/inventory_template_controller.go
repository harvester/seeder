package controllers

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubevirtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/util"
	networkingv1 "k8s.io/api/networking/v1"
)

type InventoryTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logr.Logger
}

// reconcile InventoryTemplate objects
func (r *InventoryTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	i := &seederv1alpha1.InventoryTemplate{}
	err := r.Get(ctx, req.NamespacedName, i)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.Error(err, "unable to fetch inventorytemplate object")
		return ctrl.Result{}, err
	}

	if i.DeletionTimestamp.IsZero() {
		// ensure a finalizer exists
		if !controllerutil.ContainsFinalizer(i, seederv1alpha1.InventoryTemplateFinalizer) {
			controllerutil.AddFinalizer(i, seederv1alpha1.InventoryTemplateFinalizer)
			return ctrl.Result{}, r.Update(ctx, i)
		}
		// reconcile object and ensure associated VirtualMachinePool exists
	} else {
		return ctrl.Result{}, r.CleanupMachines(ctx, i)
	}
	return ctrl.Result{}, r.EnsureMachines(ctx, i)
}

// SetupWithManager sets up the controller with the Manager.
func (r *InventoryTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&seederv1alpha1.InventoryTemplate{}).
		Watches(&seederv1alpha1.Inventory{}, &handler.EnqueueRequestForObject{}).
		Named("inventorytemplates").
		Complete(r)
}

// CleanupMachines will delete all associated VM's when the InventoryTemplate object is deleted
// once all expected machines are successfully deleted we can remove the finalizer
// to ensure inventorytemplate object can be safely removed
func (r *InventoryTemplateReconciler) CleanupMachines(ctx context.Context, i *seederv1alpha1.InventoryTemplate) error {
	r.Info("cleaning up inventory template resources", "inventorytemplate", fmt.Sprintf("%s/%s", i.Namespace, i.Name))
	harvesterClient, err := util.GenerateRemoteKubeconfigFromSecret(ctx, i.Spec.Credentials, r.Client)
	if err != nil {
		return fmt.Errorf("error generating remote harvester cluster client: %w", err)
	}

	// clean up VM's on remote cluster
	err = harvesterClient.DeleteAllOf(ctx, &kubevirtv1.VirtualMachine{}, client.InNamespace(i.Spec.VMSpec.Namespace), client.MatchingLabels{seederv1alpha1.InventoryUUIDLabelKey: string(i.GetUID())})
	if err != nil {
		return fmt.Errorf("error listing vms from remote harvester cluster: %w", err)
	}

	// clean up ingress related to kubevirtbmc on remote cluster
	err = harvesterClient.DeleteAllOf(ctx, &networkingv1.Ingress{}, client.InNamespace(seederv1alpha1.KubeBMCNS), client.MatchingLabels{seederv1alpha1.InventoryUUIDLabelKey: string(i.GetUID())})
	if err != nil {
		return fmt.Errorf("error listing vms from remote harvester cluster: %w", err)
	}

	// no cleanup of inventory / auth secret is needed as it is controlled by owner references
	// these objects already contain inventorytemplate owner reference as a resulting deletion
	// of inventory template triggers their cleanup
	iObj := i.DeepCopy()
	if controllerutil.RemoveFinalizer(iObj, seederv1alpha1.InventoryTemplateFinalizer) {
		return r.Patch(ctx, iObj, client.MergeFrom(i))
	}

	// no changes needed to inventoryTemplate so can now be ignored
	return nil
}

// EnsureMachines will reconcile a VirtualMachinePool is generated for each InventoryTemplate
// kubevirt will subsequently ensure the desired VirtualMachine replicas exist for the template
// the machine of the machinepool will be inventoryTemplate.Name-inventoryTemplate.Namespace
// this is to ensure uniqueness of vm pools as the target namespace is controlled by a field in the
// VMSpec
func (r *InventoryTemplateReconciler) EnsureMachines(ctx context.Context, i *seederv1alpha1.InventoryTemplate) error {
	// generatePoolTemplate
	vmObjs, err := util.GenerateVMPool(i)
	if err != nil {
		r.Error(err, "error generating virtualmachine objects")
		return r.updateStatus(ctx, i, seederv1alpha1.InventoryTemplateProvisioningError, err.Error())
	}

	// generate remote harvester client
	harvesterClient, err := util.GenerateRemoteKubeconfigFromSecret(ctx, i.Spec.Credentials, r.Client)
	if err != nil {
		r.Error(err, "error generating remote harvester client")
		return r.updateStatus(ctx, i, seederv1alpha1.InventoryTemplateProvisioningError, err.Error())
	}

	if err := createVMObjects(ctx, harvesterClient, vmObjs); err != nil {
		r.Error(err, "error creating remote virtual machines")
		return r.updateStatus(ctx, i, seederv1alpha1.InventoryTemplateProvisioningError, err.Error())
	}

	// generate and create inventory secret for local cluster
	secret := util.GenerateTemplateSecret(i)
	if err := reconcileObject(ctx, r.Client, secret); err != nil {
		r.Error(err, "error creating kubevirtbmc secret in local cluster")
		return r.updateStatus(ctx, i, seederv1alpha1.InventoryTemplateProvisioningError, err.Error())
	}

	// find rancher-expose ip address from remote harvester cluster
	endpoint, err := util.GetIngressEndpoint(ctx, harvesterClient)
	if err != nil {
		r.Error(err, "error getting ingress endpoint from remote harvester cluster")
		return r.updateStatus(ctx, i, seederv1alpha1.InventoryTemplateProvisioningError, err.Error())
	}

	// generate ingress objects for remote cluster and inventory objects for local cluster
	ingressObjects, inventoryObjects := util.GenerateIngressAndInventoryPool(vmObjs, endpoint, i)

	// create ingress objects in remote harvester cluster
	if err := createIngressObjects(ctx, harvesterClient, ingressObjects); err != nil {
		r.Error(err, "error creating ingress objects in remote harvester cluster")
		return r.updateStatus(ctx, i, seederv1alpha1.InventoryTemplateProvisioningError, err.Error())
	}

	// create inventory objects in local cluster
	if err := createInventoryObjects(ctx, r.Client, inventoryObjects); err != nil {
		r.Error(err, "error creating inventory objects in local cluster")
		return r.updateStatus(ctx, i, seederv1alpha1.InventoryTemplateProvisioningError, err.Error())
	}

	// need to find rancher-expose ip address to generate ingress objects
	// all objects successfully provisioned
	return r.updateStatus(ctx, i, seederv1alpha1.InventoryTemplateProvisioned, "")
}

func createVMObjects(ctx context.Context, harvesterClient client.Client, vmObjs []*kubevirtv1.VirtualMachine) error {
	/*for _, vm := range vmObjs {
		vmObj := &kubevirtv1.VirtualMachine{}
		if err := harvesterClient.Get(ctx, types.NamespacedName{Name: vm.Name, Namespace: vm.Namespace}, vmObj); err != nil {
			if apierrors.IsNotFound(err) {
				if createError := harvesterClient.Create(ctx, vm); createError != nil {
					return fmt.Errorf("error creating virtualmachine: %w", createError)
				}
			}
			return fmt.Errorf("error looking up virtualmachine %s/%s in remote harvester cluster: %w", vm.Namespace, vm.Name, err)
		}
	}*/
	for _, vm := range vmObjs {
		if err := reconcileObject(ctx, harvesterClient, vm); err != nil {
			return fmt.Errorf("error applying virtualmachine object %w", err)
		}
	}
	return nil
}

func createIngressObjects(ctx context.Context, harvesterClient client.Client, ingressObjs []*networkingv1.Ingress) error {
	for _, v := range ingressObjs {
		if err := reconcileObject(ctx, harvesterClient, v); err != nil {
			return fmt.Errorf("error applying ingress object %w", err)
		}
	}
	return nil
}

func createInventoryObjects(ctx context.Context, localClient client.Client, inventoryObjs []*seederv1alpha1.Inventory) error {
	for _, v := range inventoryObjs {
		if err := reconcileObject(ctx, localClient, v); err != nil {
			return fmt.Errorf("error applying inventory object %w", err)
		}
	}
	return nil
}

// updateStatus patches the InventoryTemplate status if needed
func (r *InventoryTemplateReconciler) updateStatus(ctx context.Context, i *seederv1alpha1.InventoryTemplate, status seederv1alpha1.InventoryTemplateProvisioningStatus, msg string) error {
	iObj := i.DeepCopy()
	iObj.Status.Status = status
	iObj.Status.Message = msg
	if !reflect.DeepEqual(iObj.Status, i.Status) {
		return r.Status().Patch(ctx, iObj, client.MergeFrom(i))
	}
	return nil
}

func reconcileObject(ctx context.Context, harvesterClient client.Client, obj client.Object) error {
	var err error

	runtimeObj := obj.DeepCopyObject()
	objCopy := runtimeObj.(client.Object)

	err = harvesterClient.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, objCopy)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return harvesterClient.Create(ctx, obj)
		}
		return fmt.Errorf("error fetching object %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	/*
		unstructuredObj := &unstructured.Unstructured{}
		unstructuredObj.Object, err = runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return fmt.Errorf("error convering new object %s to unstructured object %v", obj.GetName(), err)
		}
		return harvesterClient.Patch(ctx, unstructuredObj, client.Apply, client.FieldOwner("seeder"))
	*/
	return nil
}
