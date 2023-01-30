package plugin

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/harvester/seeder/pkg/mock"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"k8s.io/apimachinery/pkg/types"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"

	"github.com/ory/dockertest/v3/docker/pkg/ioutils"

	"github.com/spf13/cobra"

	"github.com/stretchr/testify/require"
)

var (
	port        string
	docker_host string
)

const (
	token = "token"
)

func Test_CommandGenerateKubeConfig(t *testing.T) {
	ctx = context.TODO()
	assert := require.New(t)

	// setup a k3s server in docker using dockertest
	pool, err := dockertest.NewPool("")
	assert.NoError(err, "expected no error during setup of docker pool")

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

	assert.NoError(err, "expect no error creating k3s container")

	time.Sleep(60 * time.Second)
	port = k3s.GetPort("6443/tcp")
	dockerURL := os.Getenv("DOCKER_HOST")
	if dockerURL != "" {
		u, err := url.Parse(dockerURL)
		if err != nil {
			log.Fatalf("error parsing DOCKER_HOST: %v", err)
		}
		docker_host = u.Hostname()
	}

	// setup docker_host if remote docker daemon is being used

	mgmtClient, err = mock.GenerateFakeClient()
	assert.NoError(err, "expected no error during creation of mock client")

	cmd := &cobra.Command{}
	args := []string{"test-mock-cluster-not-running", "test-mock-cluster-running", "test-mock-missing-cluster"}
	tmpDir, err := ioutils.TempDir("/tmp", "gen-kubeconfig")
	assert.NoError(err, "expected no error during creation of tmpDir")
	namespace = "default"                   // all mock objects are created in mock namespace
	seederv1alpha1.DefaultAPIPrefix = "k3s" //override since we are using k3s to mock
	// patch port on clusters using annotation to allow kubeconfig to be extracted
	for _, v := range []string{"test-mock-cluster-not-running", "test-mock-cluster-running"} {
		cObj := &seederv1alpha1.Cluster{}
		err = mgmtClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: v}, cObj)
		assert.NoError(err, "expected no error while fetch mock clusters")
		if cObj.Labels == nil {
			cObj.Labels = make(map[string]string)
		}
		cObj.Labels[seederv1alpha1.OverrideAPIPortLabel] = port
		if docker_host != "" {
			cObj.Status.ClusterAddress = docker_host
		}
		cObj.Status.ClusterToken = token // update clusters token
		err = mgmtClient.Update(ctx, cObj)
		assert.NoError(err, "expected no error while patching cluster objects")
	}
	// define empty GenKubeConfig
	g := &GenKubeconfig{
		Path: tmpDir,
	}
	err = g.preflightchecks(cmd, args)
	assert.NoError(err, "expected no error during pre-flight checks")

	err = g.generateKubeConfig(cmd)
	assert.NoError(err, "expected no error during kubeconfig generation")

	// check kubeconfig exists
	_, err = os.Stat(filepath.Join(tmpDir, "test-mock-cluster-running-default.yaml"))
	assert.NoError(err, "expect to find file test-mock-cluster-running-default.yaml")
	err = os.RemoveAll(tmpDir)
	assert.NoError(err, "expect no error during clean up of tmp dir")

	pool.Purge(k3s)
}
