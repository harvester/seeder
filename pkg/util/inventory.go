package util

import (
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	"golang.org/x/net/context"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
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
func CheckAndCreateBaseBoardObject(ctx context.Context, client client.Client, log logr.Logger, instanceObj *seederv1alpha1.Inventory, schema *runtime.Scheme) error {
	b := &rufio.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instanceObj.Name,
			Namespace: instanceObj.Namespace,
		},
		Spec: *instanceObj.Spec.BaseboardManagementSpec.DeepCopy(),
	}

	controllerutil.AddFinalizer(b, seederv1alpha1.InventoryFinalizer)
	existingObj := &rufio.Machine{}
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
		existingObj.Spec = b.Spec
		return client.Update(ctx, existingObj)
	}

	return nil
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

// IsBaseboardReady checks if the BaseboardConnectivity is setup and true
func IsBaseboardReady(b *rufio.Machine) bool {
	var ready bool
	for _, c := range b.Status.Conditions {
		if c.Type == rufio.Contactable && c.Status == rufio.ConditionTrue {
			ready = true
		}
	}

	return ready
}

// ListInventory generates a list of inventory across all namespaces
func ListInventory(ctx context.Context, c client.Client) ([]seederv1alpha1.Inventory, error) {
	list := &seederv1alpha1.InventoryList{}
	err := c.List(ctx, list, &client.ListOptions{})
	if err != nil {
		return []seederv1alpha1.Inventory{}, err
	}

	return list.Items, nil
}

// ListInventoryAllocatedToCluster lists all inventory across namespaces that is allocated to a particular
// cluster
func ListInventoryAllocatedtoCluster(ctx context.Context, c client.Client, cluster *seederv1alpha1.Cluster) ([]seederv1alpha1.Inventory, error) {
	items, err := ListInventory(ctx, c)
	if err != nil {
		return []seederv1alpha1.Inventory{}, fmt.Errorf("error fetching inventory list: %v", err)
	}

	var retItems []seederv1alpha1.Inventory
	for _, v := range items {
		if v.Status.Cluster.Name == cluster.Name && v.Status.Cluster.Namespace == cluster.Namespace {
			retItems = append(retItems, v)
		}
	}

	return retItems, nil
}
