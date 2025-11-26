package webhook

import (
	"context"
	"testing"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/stretchr/testify/require"
	"github.com/tinkerbell/rufio/api/v1alpha1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	kubevirtv1 "kubevirt.io/api/core/v1"
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

func Test_verifyRemoteClusterObjects(t *testing.T) {
	assert := require.New(t)
	crdObj := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: kubevirtBMCName,
		},
	}

	nadObject := &nadv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fake",
			Namespace: "default",
		},
	}

	scObj := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "local-storage",
		},
	}

	var testCases = []struct {
		Name        string
		Objects     []client.Object
		Template    *seederv1alpha1.InventoryTemplate
		ExpectError bool
	}{
		{
			Name:    "valid inventory template object",
			Objects: []client.Object{crdObj, nadObject, scObj},
			Template: &seederv1alpha1.InventoryTemplate{
				Spec: seederv1alpha1.InventoryTemplateSpec{
					VMSpec: seederv1alpha1.VMSpec{
						Networks: []seederv1alpha1.NetworkConfig{
							{
								VMNetwork: "default/fake",
							},
						},
						Disks: []seederv1alpha1.DiskConfig{
							{
								StorageClass: "local-storage",
							},
						},
					},
				},
			},
			ExpectError: false,
		},
		{
			Name:    "invalid network name",
			Objects: []client.Object{crdObj, nadObject, scObj},
			Template: &seederv1alpha1.InventoryTemplate{
				Spec: seederv1alpha1.InventoryTemplateSpec{
					VMSpec: seederv1alpha1.VMSpec{
						Networks: []seederv1alpha1.NetworkConfig{
							{
								VMNetwork: "default/missing",
							},
						},
						Disks: []seederv1alpha1.DiskConfig{
							{
								StorageClass: "local-storage",
							},
						},
					},
				},
			},
			ExpectError: true,
		},
		{
			Name:    "invalid storage class",
			Objects: []client.Object{crdObj, nadObject, scObj},
			Template: &seederv1alpha1.InventoryTemplate{
				Spec: seederv1alpha1.InventoryTemplateSpec{
					VMSpec: seederv1alpha1.VMSpec{
						Networks: []seederv1alpha1.NetworkConfig{
							{
								VMNetwork: "default/fake",
							},
						},
						Disks: []seederv1alpha1.DiskConfig{
							{
								StorageClass: "missing-storage",
							},
						},
					},
				},
			},
			ExpectError: true,
		},
		{
			Name:    "missing kubevirt bmc",
			Objects: []client.Object{nadObject, scObj},
			Template: &seederv1alpha1.InventoryTemplate{
				Spec: seederv1alpha1.InventoryTemplateSpec{
					VMSpec: seederv1alpha1.VMSpec{
						Networks: []seederv1alpha1.NetworkConfig{
							{
								VMNetwork: "default/fake",
							},
						},
						Disks: []seederv1alpha1.DiskConfig{
							{
								StorageClass: "local-storage",
							},
						},
					},
				},
			},
			ExpectError: true,
		},
	}

	scheme := runtime.NewScheme()
	assert.NoError(kubevirtv1.AddToScheme(scheme))
	assert.NoError(nadv1.AddToScheme(scheme))
	assert.NoError(clientgoscheme.AddToScheme(scheme))
	assert.NoError(apiextensionsv1.AddToScheme(scheme))

	for _, tc := range testCases {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tc.Objects...).Build()
		err := verifyRemoteClusterObjects(context.TODO(), tc.Template, fakeClient)
		if tc.ExpectError {
			assert.Error(err, "expected to find error", tc.Name)
		} else {
			assert.Nil(err, "expected to not find error", tc.Name)
		}
	}
}
