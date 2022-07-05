package util

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/ory/dockertest"
	"github.com/ory/dockertest/docker"
	"github.com/stretchr/testify/assert"
	typedCore "k8s.io/client-go/kubernetes/typed/core/v1"
)

var port string

const (
	token = "token"
)

func TestMain(t *testing.M) {
	// setup a k3s server in docker using dockertest
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatal(err)
	}

	runOpts := &dockertest.RunOptions{
		Name:       "k3s-mock",
		Repository: "rancher/k3s",
		Tag:        "v1.24.2-k3s1",
		Cmd:        []string{"server"},
		Env: []string{
			fmt.Sprintf("K3S_TOKEN=%s", token),
		},
		Mounts: []string{
			"tmpfs:/run",
			"tmpfs:/var/run",
		},
		Privileged: true,
		ExposedPorts: []string{
			"6443/tcp",
		},
	}

	k3s, err := pool.RunWithOptions(runOpts, func(config *docker.HostConfig) {
		// set AutoRemove to true so that stopped container goes away by itself
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{
			Name: "no",
		}
	})

	if err != nil {
		log.Fatal(err)
	}

	time.Sleep(60 * time.Second)
	port = k3s.GetPort("6443/tcp")
	code := t.Run()
	pool.Purge(k3s)
	os.Exit(code)

}

func Test_FetchKubeConfig(t *testing.T) {
	resp, err := FetchKubeConfig(fmt.Sprintf("https://localhost:%s", port), "k3s",
		token)
	assert.NoError(t, err, "expected no error while fetching kubeconfig")
	k8sclient, err := clientcmd.NewClientConfigFromBytes(resp)
	assert.NoError(t, err, "expected no error while generating k8sclient")
	rest, err := k8sclient.ClientConfig()
	assert.NoError(t, err, "expected no error while generating rest.Config")
	typedClient, err := typedCore.NewForConfig(rest)
	assert.NoError(t, err, "expected no error while generating typedClient")
	nodes, err := typedClient.Nodes().List(context.TODO(), metav1.ListOptions{})
	assert.NoError(t, err, "error listing nodes")
	assert.Len(t, nodes.Items, 1, "expected to find 1 node in the cluster")
}
