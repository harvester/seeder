package util

import (
	"context"
	bmaasv1alpha1 "github.com/harvester/bmaas/pkg/api/v1alpha1"
	"github.com/harvester/bmaas/pkg/mock"
	"github.com/stretchr/testify/assert"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

var (
	ctx = context.TODO()
	l   = log.FromContext(ctx)
)

// Test_CheckSecretExists tests the CheckSecretExists utility function
func Test_CheckSecretExists(t *testing.T) {
	c, err := mock.GenerateFakeClient()

	assert.Equal(t, nil, err, "error creating mock client")

	err = CheckSecretExists(ctx, c, l, v1.SecretReference{Name: "fiftytwo", Namespace: "default"})
	assert.Equal(t, nil, err, "error looking up secret")
}

// Test_CheckSecretExists tests the CheckSecretExists utility function
func Test_CheckSecretExistsFailure(t *testing.T) {
	c, err := mock.GenerateFakeClient()

	assert.Equal(t, nil, err, "error creating mock client")

	err = CheckSecretExists(ctx, c, l, v1.SecretReference{Name: "fiftythree", Namespace: "default"})
	assert.NotEqual(t, nil, err, "error looking up secret")
}

// Test_CheckAndCreateBaseBoardObject tests the successful creation of a baseboard object
func Test_CheckAndCreateBaseBoardObject(t *testing.T) {
	c, err := mock.GenerateFakeClient()
	assert.Equal(t, nil, err, "error creating mock client")

	i := &bmaasv1alpha1.Inventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fiftyone",
			Namespace: "default",
		},
		Spec: bmaasv1alpha1.InventorySpec{
			PrimaryDisk: "/dev/sda1",
			PXEBootInterface: bmaasv1alpha1.PXEBootInterface{
				MacAddress: "xx:xx:xx:xx:xx",
			},
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
	assert.Equal(t, nil, err, "error creating baseboard object")
}

func Test_CheckAndCreateBaseBoardObjectFailure(t *testing.T) {
	c, err := mock.GenerateFakeClient()
	assert.Equal(t, nil, err, "error creating mock client")

	i := &bmaasv1alpha1.Inventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fiftythree",
			Namespace: "default",
		},
		Spec: bmaasv1alpha1.InventorySpec{
			PrimaryDisk: "/dev/sda1",
			PXEBootInterface: bmaasv1alpha1.PXEBootInterface{
				MacAddress: "xx:xx:xx:xx:xx",
			},
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
	assert.Equal(t, nil, err, "error creating baseboard object")
}
