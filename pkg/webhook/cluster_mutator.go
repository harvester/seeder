package webhook

import (
	"context"
	"strings"

	werror "github.com/harvester/webhook/pkg/error"
	"github.com/harvester/webhook/pkg/server/admission"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

type ClusterMutator struct {
	client client.Client
	ctx    context.Context
	admission.DefaultMutator
}

func NewClusterMutator(ctx context.Context, mgr manager.Manager) *ClusterMutator {
	return &ClusterMutator{
		client: mgr.GetClient(),
		ctx:    ctx,
	}
}

func (c *ClusterMutator) Resource() admission.Resource {
	return admission.Resource{
		Names:      []string{"clusters"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   seederv1alpha1.GroupVersion.Group,
		APIVersion: seederv1alpha1.GroupVersion.Version,
		ObjectType: &seederv1alpha1.Cluster{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
		},
	}
}

func (c *ClusterMutator) Create(req *admission.Request, newObj runtime.Object) (admission.Patch, error) {
	clusterObj, ok := newObj.(*seederv1alpha1.Cluster)
	if !ok {
		return nil, werror.NewBadRequest("unable to assert object to Cluster Object")
	}
	var patchOps admission.Patch
	userName := req.UserInfo.Username

	// if there is an existing owner reference, then replace it with details in request
	labels := clusterObj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	labels[seederv1alpha1.ClusterOwnerKey] = userName
	val, ok := req.UserInfo.Extra[seederv1alpha1.ExtraFieldKey]
	if ok {
		labels[seederv1alpha1.ClusterOwnerDetailsKey] = strings.Join(val, " ")
	}

	patchOps = append(patchOps, admission.PatchOp{
		Op:    admission.PatchOpAdd,
		Path:  "/metadata/labels",
		Value: labels,
	})
	return patchOps, nil
}
