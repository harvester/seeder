package util

import (
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"testing"

	"github.com/stretchr/testify/assert"
	typedCore "k8s.io/client-go/kubernetes/typed/core/v1"
)

func Test_FetchKubeConfig(t *testing.T) {
	resp, err := FetchKubeConfig("https://localhost:39101", "k3s",
		"token")
	assert.NoError(t, err, "expected no error while fetching kubeconfig")
	t.Log(string(resp))
	k8sclient, err := clientcmd.NewClientConfigFromBytes(resp)
	assert.NoError(t, err, "expected no error while generating k8sclient")
	rest, err := k8sclient.ClientConfig()
	assert.NoError(t, err, "expected no error while generating rest.Config")
	typedClient, err := typedCore.NewForConfig(rest)
	assert.NoError(t, err, "expected no error while generating typedClient")
	nodes, err := typedClient.Nodes().List(context.TODO(), metav1.ListOptions{})
	assert.NoError(t, err, "error listing nodes")
	t.Log(nodes)
}
