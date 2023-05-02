package util

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/harvester/seeder/pkg/mock"
)

func Test_setupEmbeddedCluster(t *testing.T) {
	assert := require.New(t)
	c, err := mock.GenerateFakeClient()
	assert.NoError(err, "expected no error during creation of mock client")
	err = SetupLocalCluster(ctx, c)
	assert.NoError(err, "expected no error during creation of local cluster")
}
