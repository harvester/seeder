package webhook

import (
	"context"
	"encoding/json"
	"testing"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/webhook/pkg/server/admission"
	"github.com/rancher/wrangler/pkg/webhook"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	clusterObj = &seederv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
	}

	userName = "dev"

	request = &admission.Request{
		Request: &webhook.Request{
			AdmissionRequest: v1.AdmissionRequest{
				UserInfo: authenticationv1.UserInfo{
					Username: userName,
				},
			},
		},
	}
)

func Test_ClusterMutation(t *testing.T) {
	type testCases struct {
		name                  string
		generateClusterObject func(*seederv1alpha1.Cluster) *seederv1alpha1.Cluster
	}

	var cases = []testCases{
		{
			name:                  "no cluster owner defined",
			generateClusterObject: func(cluster *seederv1alpha1.Cluster) *seederv1alpha1.Cluster { return cluster },
		},
		{
			name: "cluster object has an owner defined",
			generateClusterObject: func(cluster *seederv1alpha1.Cluster) *seederv1alpha1.Cluster {
				clusterCopy := cluster.DeepCopy()
				clusterCopy.Labels = map[string]string{
					seederv1alpha1.ClusterOwnerKey: "demo",
				}
				return clusterCopy
			},
		},
	}

	for _, testCase := range cases {
		assert := require.New(t)
		scheme := runtime.NewScheme()
		err := seederv1alpha1.AddToScheme(scheme)
		assert.NoError(err, testCase.name)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(testCase.generateClusterObject(clusterObj)).Build()
		c := &ClusterMutator{
			client: fakeClient,
			ctx:    context.TODO(),
		}
		ops, err := c.Create(request, clusterObj)
		assert.NoError(err, testCase.name)
		assert.Len(ops, 1, testCase.name)
		// test out by patching the object
		opsByte, err := json.Marshal(ops)
		assert.NoError(err, testCase.name)
		err = c.client.Patch(context.TODO(), clusterObj, client.RawPatch(types.JSONPatchType, opsByte))
		assert.NoError(err, testCase.name)
		updatedObj := &seederv1alpha1.Cluster{}
		err = c.client.Get(context.TODO(), types.NamespacedName{Name: clusterObj.Name, Namespace: clusterObj.Namespace}, updatedObj)
		assert.NoError(err, testCase.name)
		assert.Equal(updatedObj.GetLabels()[seederv1alpha1.ClusterOwnerKey], userName, testCase.name)
	}
}
