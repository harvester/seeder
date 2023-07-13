package webhook

import (
	"context"
	"fmt"

	werror "github.com/harvester/webhook/pkg/error"
	"github.com/harvester/webhook/pkg/server/admission"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

type InventoryValidator struct {
	client client.Client
	ctx    context.Context
	admission.DefaultValidator
}

func NewInventoryValidatory(ctx context.Context, mgr manager.Manager) *InventoryValidator {
	return &InventoryValidator{
		client: mgr.GetClient(),
		ctx:    ctx,
	}
}

func (iv *InventoryValidator) Resource() admission.Resource {
	return admission.Resource{
		Names:      []string{"inventories"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   seederv1alpha1.GroupVersion.Group,
		APIVersion: seederv1alpha1.GroupVersion.Version,
		ObjectType: &seederv1alpha1.Inventory{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (iv *InventoryValidator) Create(request *admission.Request, newObj runtime.Object) error {
	return iv.validateInventory(newObj)
}

func (iv *InventoryValidator) Update(request *admission.Request, oldObj runtime.Object, newObj runtime.Object) error {
	return iv.validateInventory(newObj)
}

func (iv *InventoryValidator) validateInventory(newObj runtime.Object) error {
	iObj, ok := newObj.(*seederv1alpha1.Inventory)
	if !ok {
		return werror.NewBadRequest("unable to assert object to Inventory Object")
	}

	return iv.identifyDuplicateInventorySpec(iObj)
}

func (iv *InventoryValidator) identifyDuplicateInventorySpec(iObj *seederv1alpha1.Inventory) error {
	inventoryList := &seederv1alpha1.InventoryList{}
	err := iv.client.List(iv.ctx, inventoryList)
	if err != nil {
		return err
	}

	for _, v := range inventoryList.Items {

		// ignore self from list
		if iObj.Name == v.Name && iObj.Namespace == v.Namespace {
			continue
		}

		if v.Spec.BaseboardManagementSpec.Connection.Host == iObj.Spec.BaseboardManagementSpec.Connection.Host && v.Spec.BaseboardManagementSpec.Connection.Port == iObj.Spec.BaseboardManagementSpec.Connection.Port {
			return apierrors.NewBadRequest(fmt.Sprintf("inventory object %s exists with same baseboard host and port", v.Name))
		}
	}
	return nil
}
