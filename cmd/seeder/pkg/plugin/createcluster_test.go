package plugin

import (
	"testing"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/harvester/seeder/pkg/mock"
	"github.com/stretchr/testify/require"

	"github.com/spf13/cobra"
)

func Test_CommandCreateClusterPass(t *testing.T) {
	var err error
	cmd := &cobra.Command{}
	inv := []string{"inventory-1"}
	addPool := "mock-pool"
	namespace = "default"
	imageURL := "http://localhost/iso"

	c := &CreateCluster{
		Version:     "v1.0.3",
		Inventory:   inv,
		AddressPool: addPool,
		ImageURL:    imageURL,
	}
	assert := require.New(t)
	mgmtClient, err = mock.GenerateFakeClient()
	assert.NoError(err, "expected no error during generation of mock client")
	err = c.preflightchecks(cmd, []string{"mock-cluster"})
	assert.NoError(err, "expected no error during preflightchecks")

	err = c.createCluster(cmd)
	assert.NoError(err, "expected no error during cluster creation")
	clusterObj := &seederv1alpha1.Cluster{}
	err = mgmtClient.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: namespace}, clusterObj)
	assert.NoError(err, "expect no error looking up cluster")
	assert.Equal(addPool, clusterObj.Spec.VIPConfig.AddressPoolReference.Name, "expected vip addresspools to match")
	assert.Len(clusterObj.Spec.Nodes, 1, "expected to find one node")
	assert.Equal(addPool, clusterObj.Spec.Nodes[0].AddressPoolReference.Name, "expected node address pools to match")
}

func Test_CommandCreateClusterMissingInventory(t *testing.T) {
	var err error
	cmd := &cobra.Command{}
	inv := []string{"inventory-3"}
	addPool := "mock-pool"
	namespace = "default"
	imageURL := "http://localhost/iso"

	c := &CreateCluster{
		Version:     "v1.0.3",
		Inventory:   inv,
		AddressPool: addPool,
		ImageURL:    imageURL,
	}
	assert := require.New(t)
	mgmtClient, err = mock.GenerateFakeClient()
	assert.NoError(err, "expected no error during generation of mock client")
	err = c.preflightchecks(cmd, []string{"mock-cluster"})
	assert.Error(err, "expected no error during preflightchecks")
	assert.ErrorIs(err, createClusterPreflightError)
}
