/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	dockertest "github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	tinkv1alpha1 "github.com/tinkerbell/tink/pkg/apis/core/v1alpha1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	log "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/crd"
	"github.com/harvester/seeder/pkg/mock"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	k8sClient      client.Client
	testEnv        *envtest.Environment
	ctx            context.Context
	cancel         context.CancelFunc
	pool           *dockertest.Pool
	redfishMock    *dockertest.Resource
	k3sMock        *dockertest.Resource
	k3sNodeAddress string
	k3sNodeGateway string
	redfishAddress string
)

const (
	defaultToken = "token"
	k3sPort      = "6443"
	redfishPort  = "8000"
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	suiteConfig, _ := GinkgoConfiguration()
	_, ok := os.LookupEnv("SKIPINDRONE")
	if ok {
		suiteConfig.LabelFilter = "!skip-in-drone"
	}
	RunSpecs(t,
		"Controller Suite",
		suiteConfig,
	)
}

var _ = BeforeSuite(func() {
	log.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx, cancel = context.WithCancel(context.TODO())
	By("bootstrapping test environment")
	testEnv = &envtest.Environment{}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// install CRD's
	err = crd.Create(ctx, cfg)
	Expect(err).NotTo(HaveOccurred())
	err = seederv1alpha1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	err = rufio.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	err = clientgoscheme.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	err = tinkv1alpha1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())
	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme,
		Port:               9444,
		MetricsBindAddress: ":9080",
		LeaderElection:     false,
	})
	Expect(err).NotTo(HaveOccurred())

	err = (&InventoryReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Logger: log.Log.WithName("controller.inventory"),
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	err = (&mock.FakeBaseboardReconciller{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Logger: log.Log.WithName("controller.baseboard"),
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	err = (&mock.FakeBaseboardJobReconciller{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Logger: log.Log.WithName("controller.bmcjob"),
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	err = (&AddressPoolReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Logger: log.Log.WithName("controller.addresspool"),
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	err = (&ClusterReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Logger: log.Log.WithName("controller.cluster"),
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	err = (&InventoryEventReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Logger:        log.Log.WithName("controller.invenory-event"),
		EventRecorder: mgr.GetEventRecorderFor("seeder"),
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	err = (&ClusterEventReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Logger: log.Log.WithName("controller.cluster-event"),
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	err = (&LocalClusterReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Logger:        log.Log.WithName("controller.local-cluster"),
		EventRecorder: mgr.GetEventRecorderFor("seeder"),
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err := mgr.Start(ctx)
		Expect(err).ToNot(HaveOccurred())
	}()

	pool, err = dockertest.NewPool("")
	Expect(err).NotTo(HaveOccurred())

	redfishBuildOpts := &dockertest.BuildOptions{
		ContextDir: "../events/testdata",
	}
	redfishRunOpts := &dockertest.RunOptions{
		Name: "redfishmock",
		Cmd: []string{
			"-D",
			"/mockup",
			"--ssl",
			"--cert",
			"/mockup/localhost.crt",
			"--key",
			"/mockup/localhost.key",
		},
	}

	redfishMock, err = pool.BuildAndRunWithBuildOptions(redfishBuildOpts, redfishRunOpts)
	Expect(err).NotTo(HaveOccurred())

	k3sRunOpts := &dockertest.RunOptions{
		Name:       "k3s-mock",
		Repository: "rancher/k3s",
		Tag:        "v1.24.2-k3s1",
		Cmd:        []string{"server", "--cluster-init"},
		Env: []string{
			fmt.Sprintf("K3S_TOKEN=%s", defaultToken),
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

	k3sMock, err = pool.RunWithOptions(k3sRunOpts, func(config *docker.HostConfig) {
		// set AutoRemove to true so that stopped container goes away by itself
		config.RestartPolicy = docker.RestartPolicy{
			Name: "no",
		}
	})
	Expect(err).ToNot(HaveOccurred())

	networks, err := pool.NetworksByName("bridge")
	Expect(err).ToNot(HaveOccurred())
	Expect(len(networks)).To(Equal(1))

	time.Sleep(30 * time.Second)
	k3sNodeAddress = k3sMock.GetIPInNetwork(&networks[0])
	k3sNodeGateway = networks[0].Network.IPAM.Config[0].Gateway
	redfishAddress = redfishMock.GetIPInNetwork(&networks[0])
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := pool.Purge(redfishMock)
	Expect(err).NotTo(HaveOccurred())
	err = pool.Purge(k3sMock)
	Expect(err).NotTo(HaveOccurred())
	err = testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
