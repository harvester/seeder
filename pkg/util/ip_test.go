package util

import (
	bmaasv1alpha1 "github.com/harvester/bmaas/pkg/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

var (
	testPool = &bmaasv1alpha1.AddressPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testpool",
			Namespace: "default",
		},
		Spec: bmaasv1alpha1.AddressSpec{
			CIDR:    "192.168.1.1/29",
			Gateway: "192.168.1.7",
		},
	}
)

func Test_GenerateAddressPoolStatus(t *testing.T) {
	status, err := GenerateAddressPoolStatus(testPool)
	assert.NoError(t, err, "expected no error to have occured during address pool status generation")
	assert.Equal(t, status.AvailableAddresses, 7)
	assert.Equal(t, status.StartAddress, "192.168.1.0")
	assert.Equal(t, status.LastAddress, "192.168.1.7")
	assert.Equal(t, status.Status, bmaasv1alpha1.PoolReady)
}

func Test_AllocateAddress(t *testing.T) {
	status, err := GenerateAddressPoolStatus(testPool)
	assert.NoError(t, err, "expected no error to have occured during address pool status generation")
	address, err := AllocateAddress(status, "")
	assert.NoError(t, err, "expected no error during address allocation")
	assert.NotEmpty(t, address, "generated address should not have been empty")
	status.AddressAllocation = map[string]bmaasv1alpha1.ObjectReference{
		address: {Namespace: "default", Name: "demo"},
	}
	_, err = AllocateAddress(status, address)
	assert.Error(t, err, "expected error allocating same address twice")
}

func Test_DeallocateAddress(t *testing.T) {
	status, err := GenerateAddressPoolStatus(testPool)
	assert.NoError(t, err, "expected no error to have occured during address pool status generation")
	address, err := AllocateAddress(status, "")
	assert.NoError(t, err, "expected no error during address allocation")
	assert.NotEmpty(t, address, "generated address should not have been empty")
	status.AddressAllocation = map[string]bmaasv1alpha1.ObjectReference{
		address: {Namespace: "default", Name: "demo"},
	}
	err = DeallocateAddress(status, address)
	assert.NoError(t, err, "expected no error while removing ip address")
	assert.Empty(t, len(status.AddressAllocation), "expected no addresses to be allocated")
}
