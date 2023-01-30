package plugin

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"

	"github.com/harvester/seeder/pkg/mock"

	"github.com/spf13/cobra"

	"github.com/stretchr/testify/require"
)

func Test_RecreateCluster(t *testing.T) {
	assert := require.New(t)
	ctx = context.TODO()
	namespace = "default"
	var err error
	mgmtClient, err = mock.GenerateFakeClient()
	assert.NoError(err, "expected no error setting up mock client")

	clusterName = "test-mock-cluster-running"
	rc := &cobra.Command{}
	r := &RecreateCluster{
		Version: "v1.1.1",
	}

	exists, err := r.clusterExists(rc)
	assert.NoError(err, "expected no error while looking up cluster")
	assert.True(exists, "expected cluster to exist")

	err = r.deleteCluster(rc)
	assert.NoError(err, "expected no error deleting cluster")

	err = r.recreateCluster(rc)
	assert.NoError(err, "expected no error recreating cluster")

	clusterObj := &seederv1alpha1.Cluster{}
	err = mgmtClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: clusterName}, clusterObj)
	assert.NoError(err, "expected no error looking up cluster")
	assert.Equal("v1.1.1", clusterObj.Spec.HarvesterVersion, "expected cluster object version to match")

}
