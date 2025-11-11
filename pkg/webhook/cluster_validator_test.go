package webhook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

var (
	cluster1 = &seederv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: "default",
		},
		Spec: seederv1alpha1.ClusterSpec{
			Nodes: []seederv1alpha1.NodeConfig{
				{
					InventoryReference: seederv1alpha1.ObjectReference{
						Name:      "node1",
						Namespace: "default",
					},
				},
				{
					InventoryReference: seederv1alpha1.ObjectReference{
						Name:      "node2",
						Namespace: "default",
					},
				},
				{
					InventoryReference: seederv1alpha1.ObjectReference{
						Name:      "node3",
						Namespace: "default",
					},
				},
			},
		},
	}

	cluster2 = &seederv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster2",
			Namespace: "default",
		},
		Spec: seederv1alpha1.ClusterSpec{
			Nodes: []seederv1alpha1.NodeConfig{
				{
					InventoryReference: seederv1alpha1.ObjectReference{
						Name:      "node4",
						Namespace: "non-default",
					},
				},
				{
					InventoryReference: seederv1alpha1.ObjectReference{
						Name:      "node5",
						Namespace: "non-default",
					},
				},
				{
					InventoryReference: seederv1alpha1.ObjectReference{
						Name:      "node6",
						Namespace: "non-default",
					},
				},
			},
		},
	}

	clusterList = []client.Object{cluster1, cluster2}
)

func Test_checkInventoryIsFree(t *testing.T) {
	type testCases struct {
		Name          string
		InputCluster  *seederv1alpha1.Cluster
		ErrorExpected bool
	}

	cases := []testCases{
		{
			Name: "all nodes free",
			InputCluster: &seederv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-cluster",
					Namespace: "default",
				},
				Spec: seederv1alpha1.ClusterSpec{
					Nodes: []seederv1alpha1.NodeConfig{
						{
							InventoryReference: seederv1alpha1.ObjectReference{
								Name:      "node4",
								Namespace: "default",
							},
						},
						{
							InventoryReference: seederv1alpha1.ObjectReference{
								Name:      "node5",
								Namespace: "default",
							},
						},
						{
							InventoryReference: seederv1alpha1.ObjectReference{
								Name:      "node6",
								Namespace: "default",
							},
						},
					},
				},
			},
			ErrorExpected: false,
		},
		{
			Name: "all nodes occupied",
			InputCluster: &seederv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-cluster",
					Namespace: "default",
				},
				Spec: seederv1alpha1.ClusterSpec{
					Nodes: []seederv1alpha1.NodeConfig{
						{
							InventoryReference: seederv1alpha1.ObjectReference{
								Name:      "node1",
								Namespace: "default",
							},
						},
						{
							InventoryReference: seederv1alpha1.ObjectReference{
								Name:      "node2",
								Namespace: "default",
							},
						},
						{
							InventoryReference: seederv1alpha1.ObjectReference{
								Name:      "node3",
								Namespace: "default",
							},
						},
					},
				},
			},
			ErrorExpected: true,
		},
		{
			Name: "some nodes free, some nodes occupied",
			InputCluster: &seederv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-cluster",
					Namespace: "default",
				},
				Spec: seederv1alpha1.ClusterSpec{
					Nodes: []seederv1alpha1.NodeConfig{
						{
							InventoryReference: seederv1alpha1.ObjectReference{
								Name:      "node1",
								Namespace: "non-default",
							},
						},
						{
							InventoryReference: seederv1alpha1.ObjectReference{
								Name:      "node2",
								Namespace: "non-default",
							},
						},
						{
							InventoryReference: seederv1alpha1.ObjectReference{
								Name:      "node3",
								Namespace: "default",
							},
						},
					},
				},
			},
			ErrorExpected: true,
		},
	}

	assert := require.New(t)

	scheme := runtime.NewScheme()
	err := seederv1alpha1.AddToScheme(scheme)
	assert.NoError(err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(clusterList...).Build()
	cv := &ClusterValidator{
		client: fakeClient,
		ctx:    context.TODO(),
	}

	for _, testCase := range cases {
		err := cv.checkInventoryIsFree(testCase.InputCluster)
		if testCase.ErrorExpected {
			assert.Errorf(err, "expected to find error for case: %s", testCase.Name)
		} else {
			assert.NoErrorf(err, "expected to find no error for case: %s", testCase.Name)
		}
	}
}
