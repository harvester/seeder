package util

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	dockertest "github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typedCore "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
)

var k3sNodeAddress string

const (
	token   = "token"
	k3sPort = "6443"
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
		Cmd:        []string{"server", "--cluster-init"},
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

	networks, err := pool.NetworksByName("bridge")
	if err != nil {
		log.Fatal(err)
	}

	k3sNodeAddress = k3s.GetIPInNetwork(&networks[0])
	time.Sleep(60 * time.Second)

	// needed to pass check in controller-runtime https://github.com/kubernetes-sigs/controller-runtime/commit/ed8be90b87613a10303ff8a74e9452bb47e77bf7

	runtimelog.SetLogger(l)
	if err != nil {
		_ = pool.Purge(k3s)
		log.Fatal(err)
	}
	code := t.Run()
	_ = pool.Purge(k3s)
	os.Exit(code)

}

func Test_GenerateKubeConfig(t *testing.T) {
	assert := require.New(t)
	c, err := GenerateKubeConfig(k3sNodeAddress, k3sPort, "k3s", token)
	assert.NoError(err, "expected no error during generation of kubeconfig")
	assert.NoError(err)
	k8sclient, err := clientcmd.NewClientConfigFromBytes(c)
	assert.NoError(err, "expected no error while generating k8sclient")
	rest, err := k8sclient.ClientConfig()
	assert.NoError(err, "expected no error while generating rest.Config")
	typedClient, err := typedCore.NewForConfig(rest)
	assert.NoError(err, "expected no error while generating typedClient")
	nodes, err := typedClient.Nodes().List(context.TODO(), metav1.ListOptions{})
	assert.NoError(err, "error listing nodes")
	assert.Len(nodes.Items, 1, "expected to find 1 node in the cluster")
	node := nodes.Items[0]
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}
	node.Labels["random"] = "test"
	_, err = typedClient.Nodes().Update(ctx, &node, metav1.UpdateOptions{})
	assert.NoError(err, "expected no error while updating node")
}
