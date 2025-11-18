package webhook

import (
	"context"
	"fmt"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	werror "github.com/harvester/webhook/pkg/error"
	"github.com/harvester/webhook/pkg/server/admission"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

type ClusterValidator struct {
	admission.DefaultValidator
	ctx    context.Context
	client client.Client
}

func NewClusterValidator(ctx context.Context, mgr manager.Manager) *ClusterValidator {
	return &ClusterValidator{
		ctx:    ctx,
		client: mgr.GetClient(),
	}
}

func (cv *ClusterValidator) Resource() admission.Resource {
	return admission.Resource{
		Names:      []string{"clusters"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   seederv1alpha1.GroupVersion.Group,
		APIVersion: seederv1alpha1.GroupVersion.Version,
		ObjectType: &seederv1alpha1.Cluster{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (cv *ClusterValidator) Create(request *admission.Request, newObj runtime.Object) error {
	return cv.validateCluster(newObj)
}

func (cv *ClusterValidator) Update(request *admission.Request, oldObj runtime.Object, newObj runtime.Object) error {
	return cv.validateCluster(newObj)
}

func (cv *ClusterValidator) validateCluster(newObj runtime.Object) error {
	cluster, ok := newObj.(*seederv1alpha1.Cluster)
	if !ok {
		return werror.NewBadRequest("unable to assert object to Cluster Object")
	}

	return cv.checkInventoryIsFree(cluster)
}

/* Check that the new cluster doesn't use an inventory object that is already in
 * use with an existing cluster.
 */
func (cv *ClusterValidator) checkInventoryIsFree(cluster *seederv1alpha1.Cluster) error {
	clusterList := &seederv1alpha1.ClusterList{}
	err := cv.client.List(cv.ctx, clusterList)
	if err != nil {
		return err
	}

	for _, n := range cluster.Spec.Nodes {
		for _, c := range clusterList.Items {
			// ignore self
			if c.Name == cluster.Name && c.Namespace == cluster.Namespace {
				continue
			}

			for _, xn := range c.Spec.Nodes {
				if isSameInventory(n.InventoryReference, xn.InventoryReference) {
					return werror.NewBadRequest(fmt.Sprintf("inventory %s/%s is already in use with cluster %s/%s", n.InventoryReference.Namespace, n.InventoryReference.Name, c.Namespace, c.Name))
				}
			}
		}
	}
	return nil
}

func isSameInventory(a, b seederv1alpha1.ObjectReference) bool {
	return a.Name == b.Name && a.Namespace == b.Namespace
}
