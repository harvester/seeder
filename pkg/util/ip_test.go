package util

import (
	"testing"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	testPool = &seederv1alpha1.AddressPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testpool",
			Namespace: "default",
		},
		Spec: seederv1alpha1.AddressSpec{
			CIDR:    "192.168.1.1/29",
			Gateway: "192.168.1.7",
		},
	}
)

func Test_GenerateAddressPoolStatus(t *testing.T) {
	assert := require.New(t)
	status, err := GenerateAddressPoolStatus(testPool)
	assert.NoError(err, "expected no error to have occured during address pool status generation")
	assert.Equal(status.AvailableAddresses, 7)
	assert.Equal(status.StartAddress, "192.168.1.0")
	assert.Equal(status.LastAddress, "192.168.1.7")
	assert.Equal(status.Status, seederv1alpha1.PoolReady)
}

func Test_AllocateAddress(t *testing.T) {
	assert := require.New(t)
	status, err := GenerateAddressPoolStatus(testPool)
	assert.NoError(err, "expected no error to have occured during address pool status generation")
	address, err := AllocateAddress(status, "")
	assert.NoError(err, "expected no error during address allocation")
	assert.NotEmpty(address, "generated address should not have been empty")
	status.AddressAllocation = map[string]seederv1alpha1.ObjectReferenceWithKind{
		address: {ObjectReference: seederv1alpha1.ObjectReference{Namespace: "default", Name: "demo"}, Kind: "inventory"},
	}
	_, err = AllocateAddress(status, address)
	assert.Error(err, "expected error allocating same address twice")
}

func Test_DeallocateAddress(t *testing.T) {
	assert := require.New(t)
	status, err := GenerateAddressPoolStatus(testPool)
	assert.NoError(err, "expected no error to have occured during address pool status generation")
	address, err := AllocateAddress(status, "")
	assert.NoError(err, "expected no error during address allocation")
	assert.NotEmpty(address, "generated address should not have been empty")
	status.AddressAllocation = map[string]seederv1alpha1.ObjectReferenceWithKind{
		address: {ObjectReference: seederv1alpha1.ObjectReference{Namespace: "default", Name: "demo"}, Kind: "inventory"},
	}
	err = DeallocateAddress(status, address)
	assert.NoError(err, "expected no error while removing ip address")
	assert.Empty(len(status.AddressAllocation), "expected no addresses to be allocated")
}
