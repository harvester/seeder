package util

import (
	"testing"

	"github.com/harvester/seeder/pkg/mock"
	"github.com/stretchr/testify/require"
)

func Test_setupEmbeddedCluster(t *testing.T) {
	assert := require.New(t)
	c, err := mock.GenerateFakeClient()
	assert.NoError(err, "expected no error during creation of mock client")
	err = SetupLocalCluster(ctx, c)
	assert.NoError(err, "expected no error during creation of local cluster")
}
