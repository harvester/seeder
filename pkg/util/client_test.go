package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_FetchKubeConfig(t *testing.T) {
	err := FetchKubeConfig("https://localhost:45211", "token")
	assert.NoError(t, err, "expected no error while fetching kubeconfig")
}
