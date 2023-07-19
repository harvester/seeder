package webhook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tinkerbell/rufio/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

var (
	iObj1 = &seederv1alpha1.Inventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "inventory1",
			Namespace: "default",
		},
		Spec: seederv1alpha1.InventorySpec{
			BaseboardManagementSpec: v1alpha1.MachineSpec{
				Connection: v1alpha1.Connection{
					Host: "endpoint1",
					Port: 623,
				},
			},
		},
	}

	iObj2 = &seederv1alpha1.Inventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "inventory2",
			Namespace: "default",
		},
		Spec: seederv1alpha1.InventorySpec{
			BaseboardManagementSpec: v1alpha1.MachineSpec{
				Connection: v1alpha1.Connection{
					Host: "endpoint2",
					Port: 623,
				},
			},
		},
	}

	iObj3 = &seederv1alpha1.Inventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "inventory3",
			Namespace: "harvester-system",
		},
		Spec: seederv1alpha1.InventorySpec{
			BaseboardManagementSpec: v1alpha1.MachineSpec{
				Connection: v1alpha1.Connection{
					Host: "endpoint3",
					Port: 623,
				},
			},
		},
	}

	objList = []client.Object{iObj1, iObj2, iObj3}
)

func Test_identifyDuplicateInventorySpec(t *testing.T) {
	type testCases struct {
		Name           string
		InputInventory *seederv1alpha1.Inventory
		ErrorExpected  bool
	}

	cases := []testCases{
		{
			Name: "unique object",
			InputInventory: &seederv1alpha1.Inventory{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "inventory-new",
					Namespace: "default",
				},
				Spec: seederv1alpha1.InventorySpec{
					BaseboardManagementSpec: v1alpha1.MachineSpec{
						Connection: v1alpha1.Connection{
							Host: "endpoint-new",
							Port: 623,
						},
					},
				},
			},
			ErrorExpected: false,
		},
		{
			Name: "duplicate object",
			InputInventory: &seederv1alpha1.Inventory{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "inventory-new",
					Namespace: "default",
				},
				Spec: seederv1alpha1.InventorySpec{
					BaseboardManagementSpec: v1alpha1.MachineSpec{
						Connection: v1alpha1.Connection{
							Host: "endpoint1",
							Port: 623,
						},
					},
				},
			},
			ErrorExpected: true,
		},
		{
			Name: "duplicate object across namespace",
			InputInventory: &seederv1alpha1.Inventory{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "inventory-new",
					Namespace: "default",
				},
				Spec: seederv1alpha1.InventorySpec{
					BaseboardManagementSpec: v1alpha1.MachineSpec{
						Connection: v1alpha1.Connection{
							Host: "endpoint3",
							Port: 623,
						},
					},
				},
			},
			ErrorExpected: true,
		},
		{
			Name: "skip self",
			InputInventory: &seederv1alpha1.Inventory{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "inventory1",
					Namespace: "default",
				},
				Spec: seederv1alpha1.InventorySpec{
					BaseboardManagementSpec: v1alpha1.MachineSpec{
						Connection: v1alpha1.Connection{
							Host: "endpoint1",
							Port: 623,
						},
					},
				},
			},
			ErrorExpected: false,
		},
	}

	assert := require.New(t)

	scheme := runtime.NewScheme()
	err := seederv1alpha1.AddToScheme(scheme)
	assert.NoError(err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objList...).Build()
	v := &InventoryValidator{
		client: fakeClient,
		ctx:    context.TODO(),
	}

	for _, testCase := range cases {
		err := v.identifyDuplicateInventorySpec(testCase.InputInventory)
		if testCase.ErrorExpected {
			assert.Errorf(err, "expected to find error for case: %s", testCase.Name)
		} else {
			assert.NoErrorf(err, "expected to find no error for case: %s", testCase.Name)
		}
	}
}
