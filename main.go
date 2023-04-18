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

package main

import (
	"flag"
	"os"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/harvester/seeder/pkg/util"

	"github.com/harvester/seeder/pkg/crd"

	"github.com/harvester/seeder/pkg/rufiojobwrapper"
	rufiocontrollers "github.com/tinkerbell/rufio/controllers"

	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	tinkv1alpha1 "github.com/tinkerbell/tink/pkg/apis/core/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/controllers"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

type controller interface {
	SetupWithManager(ctrl.Manager) error
}

const (
	defaultNamespace = "default"
)

func init() {
	utilruntime.Must(rufio.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(seederv1alpha1.AddToScheme(scheme))
	utilruntime.Must(tinkv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var leaderElectionNamespace string
	var embedMode bool
	ns, ok := os.LookupEnv("LEADER_ELECTION_NAMESPACE")
	if !ok {
		ns = defaultNamespace
	}

	// trigger custom reconcile loops when in embedded loop
	if val, ok := os.LookupEnv("SEEDER_EMBEDDED_MODE"); ok && strings.ToLower(val) == "true" {
		embedMode = true
	}

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&leaderElectionNamespace, "leader-election-namespace", ns, "The namespace to use for leader election")
	flag.BoolVar(&enableLeaderElection, "leader-elect", true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: false,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      metricsAddr,
		Port:                    9443,
		HealthProbeBindAddress:  probeAddr,
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        "28b21117.harvesterhci.io",
		LeaderElectionNamespace: leaderElectionNamespace,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()

	// create CRDs
	err = crd.Create(ctx, mgr.GetConfig())
	if err != nil {
		setupLog.Error(err, "unable to create crds")
		os.Exit(1)
	}

	var enabledControllers []controller
	var coreControllers = []controller{
		&controllers.ClusterReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
			Logger: log.FromContext(ctx).WithName("cluster-controller"),
		},
		&controllers.InventoryReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
			Logger: log.FromContext(ctx).WithName("inventory-controller"),
		},
		&controllers.ClusterEventReconciler{
			Client:        mgr.GetClient(),
			Scheme:        mgr.GetScheme(),
			Logger:        log.FromContext(ctx).WithName("cluster-event-controller"),
			EventRecorder: mgr.GetEventRecorderFor("seeder"),
		},
		rufiocontrollers.NewMachineReconciler(
			mgr.GetClient(),
			mgr.GetEventRecorderFor("machine-controller"),
			rufiocontrollers.NewBMCClientFactoryFunc(ctx),
			ctrl.Log.WithName("controller").WithName("Machine"),
		),
		rufiojobwrapper.NewRufioWrapper(ctx,
			mgr.GetClient(),
			ctrl.Log.WithName("controller").WithName("Job"),
		),
		rufiocontrollers.NewTaskReconciler(
			mgr.GetClient(),
			rufiocontrollers.NewBMCClientFactoryFunc(ctx),
		),
	}

	// embed mode doesnt need inventory events as they eventually flow into cluster events
	var nonEmbedModeControllers = []controller{
		&controllers.AddressPoolReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
			Logger: log.FromContext(ctx).WithName("addresspool-controller"),
		},
		&controllers.InventoryEventReconciller{
			Client:        mgr.GetClient(),
			Scheme:        mgr.GetScheme(),
			Logger:        log.FromContext(ctx).WithName("inventory-event-controller"),
			EventRecorder: mgr.GetEventRecorderFor("seeder"),
		},
	}

	var embedModeControllers = []controller{
		&controllers.LocalClusterReconciler{
			Client:        mgr.GetClient(),
			Scheme:        mgr.GetScheme(),
			Logger:        log.FromContext(ctx).WithName("local-cluster-controller"),
			EventRecorder: mgr.GetEventRecorderFor("seeder"),
		},
	}

	if embedMode {
		enabledControllers = append(coreControllers, embedModeControllers...)
	} else {
		enabledControllers = append(coreControllers, nonEmbedModeControllers...)
	}

	for _, v := range enabledControllers {
		if err := v.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "error starting controller")
			os.Exit(1)
		}
	}

	// need a tmp client as mgr.Client read caches are unavailable
	// until manager has been started
	if embedMode {
		tmpClient, err := client.New(mgr.GetConfig(), client.Options{
			Scheme: scheme,
		})
		if err != nil {
			setupLog.Error(err, "error creating temp client for local cluster setup")
		}
		err = util.SetupLocalCluster(ctx, tmpClient)
		if err != nil {
			setupLog.Error(err, "error setting up local cluster in embed mode")
			os.Exit(1)
		}
	}

	//+kubebuilder:scaffold:builder
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
