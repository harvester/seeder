package util

import (
	"context"
	"testing"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/mock"
	"github.com/stretchr/testify/require"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	ctx = context.TODO()
	l   = log.FromContext(ctx)
)

// Test_CheckSecretExists tests the CheckSecretExists utility function
func Test_CheckSecretExists(t *testing.T) {
	assert := require.New(t)
	c, err := mock.GenerateFakeClient()

	assert.Equal(nil, err, "error creating mock client")

	err = CheckSecretExists(ctx, c, l, v1.SecretReference{Name: "fiftytwo", Namespace: "default"})
	assert.Equal(nil, err, "error looking up secret")
}

// Test_CheckSecretExists tests the CheckSecretExists utility function
func Test_CheckSecretExistsFailure(t *testing.T) {
	assert := require.New(t)
	c, err := mock.GenerateFakeClient()

	assert.Equal(nil, err, "error creating mock client")

	err = CheckSecretExists(ctx, c, l, v1.SecretReference{Name: "fiftythree", Namespace: "default"})
	assert.NotEqual(nil, err, "error looking up secret")
}

// Test_CheckAndCreateBaseBoardObject tests the successful creation of a baseboard object
func Test_CheckAndCreateBaseBoardObject(t *testing.T) {
	assert := require.New(t)
	c, err := mock.GenerateFakeClient()
	assert.Equal(nil, err, "error creating mock client")

	i := &seederv1alpha1.Inventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fiftyone",
			Namespace: "default",
		},
		Spec: seederv1alpha1.InventorySpec{
			PrimaryDisk:                   "/dev/sda1",
			ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
			BaseboardManagementSpec: rufio.BaseboardManagementSpec{
				Connection: rufio.Connection{
					Host:        "localhost",
					Port:        623,
					InsecureTLS: true,
					AuthSecretRef: v1.SecretReference{
						Name:      "fiftyone",
						Namespace: "default",
					},
				},
			},
		},
	}

	err = CheckAndCreateBaseBoardObject(ctx, c, l, i, c.Scheme())
	assert.Equal(nil, err, "error creating baseboard object")
}

func Test_CheckAndCreateBaseBoardObjectFailure(t *testing.T) {
	assert := require.New(t)
	c, err := mock.GenerateFakeClient()
	assert.Equal(nil, err, "error creating mock client")

	i := &seederv1alpha1.Inventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fiftythree",
			Namespace: "default",
		},
		Spec: seederv1alpha1.InventorySpec{
			PrimaryDisk:                   "/dev/sda1",
			ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
			BaseboardManagementSpec: rufio.BaseboardManagementSpec{
				Connection: rufio.Connection{
					Host:        "localhost",
					Port:        623,
					InsecureTLS: true,
					AuthSecretRef: v1.SecretReference{
						Name:      "fiftythree",
						Namespace: "default",
					},
				},
			},
		},
	}

	err = CheckAndCreateBaseBoardObject(ctx, c, l, i, c.Scheme())
	assert.Equal(nil, err, "error creating baseboard object")
}
