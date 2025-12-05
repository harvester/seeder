package webhook

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/util"
	werror "github.com/harvester/webhook/pkg/error"
	"github.com/harvester/webhook/pkg/server/admission"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	kubevirtBMCName = "virtualmachinebmcs.virtualmachine.kubevirt.io"
)

type InventoryTemplateValidator struct {
	admission.DefaultValidator
	ctx    context.Context
	client client.Client
}

func NewInventoryTemplateValidtor(ctx context.Context, mgr manager.Manager) *InventoryTemplateValidator {
	return &InventoryTemplateValidator{
		ctx:    ctx,
		client: mgr.GetClient(),
	}
}

func (itv *InventoryTemplateValidator) Resource() admission.Resource {
	return admission.Resource{
		Names:      []string{"inventorytemplates"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   seederv1alpha1.GroupVersion.Group,
		APIVersion: seederv1alpha1.GroupVersion.Version,
		ObjectType: &seederv1alpha1.InventoryTemplate{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
			admissionregv1.Delete,
		},
	}
}

// Create call will verify that secret reference provided in InventoryTemplateSpec exists
// and contains the key kubeconfig
func (itv *InventoryTemplateValidator) Create(request *admission.Request, newObj runtime.Object) error {
	inventoryTemplateObj, ok := newObj.(*seederv1alpha1.InventoryTemplate)
	if !ok {
		return werror.NewBadRequest("unable to assert object to InventoryTemplate")
	}

	harvesterClient, err := util.GenerateRemoteKubeconfigFromSecret(itv.ctx, inventoryTemplateObj.Spec.Credentials, itv.client)
	if err != nil {
		return werror.NewBadRequest(fmt.Sprintf("unable to generate remote harvester client: %v", err))
	}

	return verifyRemoteClusterObjects(itv.ctx, inventoryTemplateObj, harvesterClient)
}

// InventoryTemplate spec is supposed to be immutable, so we need to block any udpates to the spec
// if user wishes to change their objects etc they should recreate a new InventoryTemplate
func (itv *InventoryTemplateValidator) Update(request *admission.Request, oldObj runtime.Object, newObj runtime.Object) error {
	newInventoryTemplateObj, ok := newObj.(*seederv1alpha1.InventoryTemplate)
	if !ok {
		return werror.NewBadRequest("unable to assert object to InventoryTemplate")
	}

	oldInventoryTemplateObj, ok := oldObj.(*seederv1alpha1.InventoryTemplate)
	if !ok {
		return werror.NewBadRequest("unable to assert object to InventoryTemplate")
	}

	if !reflect.DeepEqual(newInventoryTemplateObj.Spec, oldInventoryTemplateObj.Spec) {
		return werror.NewBadRequest("InventoryTemplate spec is immutable and cannot be updated")
	}

	return nil
}

// InventoryTemplate can be used directly or as part of a NestedCluster
// in the latter case the InventoryTemplate will contain owner references
// we need to block deletion if any owner references exist as the user
// needs to delete the owning object to trigger deletion of the InventoryTemplate
func (itv *InventoryTemplateValidator) Delete(request *admission.Request, oldObj runtime.Object) error {
	inventoryTemplateObj, ok := oldObj.(*seederv1alpha1.InventoryTemplate)
	if !ok {
		return werror.NewBadRequest("unable to assert object to InventoryTemplate")
	}

	if len(inventoryTemplateObj.OwnerReferences) > 0 {
		return werror.NewBadRequest("cannot delete InventoryTemplate object that is owned by another object")
	}

	return nil
}

func verifyRemoteClusterObjects(ctx context.Context, inventoryTemplateObj *seederv1alpha1.InventoryTemplate, harvesterClient client.Client) error {
	// * check if kubevirtbmc crd exists on remote cluster
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := harvesterClient.Get(ctx, apimachinerytypes.NamespacedName{Name: kubevirtBMCName}, crd); err != nil {
		if apierrors.IsNotFound(err) {
			return werror.NewBadRequest("kubevirt bmc is not installed in remote cluster")
		}
		return werror.NewBadRequest(fmt.Sprintf("error while looking up kubevirt bmc in remote cluster: %v", err))
	}
	// * check NAD name exists
	netAttachDefObj := &nadv1.NetworkAttachmentDefinition{}
	for _, network := range inventoryTemplateObj.Spec.VMSpec.Networks {
		elements := strings.Split(network.VMNetwork, "/")
		if len(elements) != 2 {
			return werror.NewBadRequest(fmt.Sprintf("network name %s appears invalid", network.VMNetwork))
		}
		if err := harvesterClient.Get(ctx, apimachinerytypes.NamespacedName{Name: elements[1], Namespace: elements[0]}, netAttachDefObj); err != nil {
			if apierrors.IsNotFound(err) {
				return werror.NewBadRequest(fmt.Sprintf("VM network %s not found in remote cluster", network.VMNetwork))
			}
			return werror.NewBadRequest(fmt.Sprintf("error while looking network definition in remote cluster: %v", err))
		}
	}
	// * check storage class exists
	storageClass := &storagev1.StorageClass{}
	for _, disk := range inventoryTemplateObj.Spec.VMSpec.Disks {
		if err := harvesterClient.Get(ctx, apimachinerytypes.NamespacedName{Name: disk.StorageClass}, storageClass); err != nil {
			if apierrors.IsNotFound(err) {
				return werror.NewBadRequest(fmt.Sprintf("storage class %s not found in remote cluster", disk.StorageClass))
			}
			return werror.NewBadRequest(fmt.Sprintf("error while looking up storage class in remote cluster: %v", err))
		}
	}
	return nil
}
