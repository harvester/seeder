package util

import (
	"fmt"
	"github.com/go-logr/logr"
	bmaasv1alpha1 "github.com/harvester/bmaas/pkg/api/v1alpha1"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	"golang.org/x/net/context"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// CheckSecretExists looks up the BMC reference secret to ensure it exists before submitting an object
func CheckSecretExists(ctx context.Context, client client.Client, log logr.Logger, secretRef v1.SecretReference) error {
	secret := &v1.Secret{}
	err := client.Get(ctx, types.NamespacedName{Name: secretRef.Name, Namespace: secretRef.Namespace}, secret)
	if err != nil {
		log.Error(err, "secret lookup error")
	}

	return err
}

// CheckAndCreateBaseBoardObject will take the BMCSpec and generate a baseboardobject of the same name
// and set owner references.
func CheckAndCreateBaseBoardObject(ctx context.Context, client client.Client, log logr.Logger, instanceObj *bmaasv1alpha1.Inventory, schema *runtime.Scheme) error {
	b := &rufio.BaseboardManagement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceObj.Name,
			Namespace: instanceObj.Namespace,
		},
		Spec: *instanceObj.Spec.BaseboardManagementSpec.DeepCopy(),
	}

	controllerutil.AddFinalizer(b, bmaasv1alpha1.InventoryFinalizer)
	existingObj := &rufio.BaseboardManagement{}
	err := client.Get(ctx, types.NamespacedName{Name: b.Name, Namespace: b.Namespace}, existingObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return CreateObject(ctx, client, b, instanceObj, log, schema)
		} else {
			log.Error(err, "Error looking up baseboardobject")
			return err
		}
	}

	if !reflect.DeepEqual(b.Spec, existingObj.Spec) {
		err = fmt.Errorf("existing object is not the same as provided object, erroring out")
	}

	return err
}

// CreateObject is a generate method to create an object and set ownership
func CreateObject(ctx context.Context, client client.Client, obj client.Object, owner client.Object, log logr.Logger, schema *runtime.Scheme) error {
	err := controllerutil.SetOwnerReference(owner, obj, schema)
	if err != nil {
		log.Error(err, "unable to set object ownership")
		return err
	}

	return client.Create(ctx, obj)
}
