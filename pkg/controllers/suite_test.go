/*
Copyright 2022.

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
	"path/filepath"
	"testing"
	"time"

	bmaasv1alpha1 "github.com/harvester/bmaas/pkg/api/v1alpha1"
	"github.com/harvester/bmaas/pkg/mock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	log "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	scheme    = runtime.NewScheme()
	ctx       context.Context
	cancel    context.CancelFunc
	eg        *errgroup.Group
	egctx     context.Context
	setupLog  = ctrl.Log.WithName("setup")
)

const (
	timeout = 300
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t,
		"Controller Suite",
	)
}

var _ = BeforeSuite(func() {
	log.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx, cancel = context.WithCancel(context.TODO())
	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = bmaasv1alpha1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	err = rufio.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	err = clientgoscheme.AddToScheme(scheme)
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

	go func() {
		defer GinkgoRecover()
		err := mgr.Start(ctx)
		Expect(err).ToNot(HaveOccurred())
	}()

	time.Sleep(30 * time.Second)
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
