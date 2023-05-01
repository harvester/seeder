package crd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ListAssets(t *testing.T) {
	assert := require.New(t)
	_, err := generateObjects()
	assert.NoError(err, "expected no error during object generation")
}
