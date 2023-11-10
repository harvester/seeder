package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	rufiocontrollers "github.com/tinkerbell/rufio/controller"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/crd"
	"github.com/harvester/seeder/pkg/rufiojobwrapper"
	"github.com/harvester/seeder/pkg/util"
	"github.com/harvester/seeder/pkg/webhook"
)

var (
	scheme = runtime.NewScheme()
)

const (
	defaultRufioTimeout = 30 * time.Second
)

type Server struct {
	MetricsAddress          string
	EnableLeaderElection    bool
	ProbeAddress            string
	LeaderElectionNamespace string
	EmbeddedMode            bool
	Debug                   bool
	logger                  logr.Logger
}

type controller interface {
	SetupWithManager(ctrl.Manager) error
}

func (s *Server) Start(ctx context.Context) error {
	utilruntime.Must(rufio.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(seederv1alpha1.AddToScheme(scheme))
	utilruntime.Must(tinkv1alpha1.AddToScheme(scheme))
	s.initLogs()
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: ":9080",
		},
		HealthProbeBindAddress:  s.ProbeAddress,
		LeaderElection:          s.EnableLeaderElection,
		LeaderElectionID:        "28b21117.harvesterhci.io",
		LeaderElectionNamespace: s.LeaderElectionNamespace,
	})
	if err != nil {
		logrus.Error(err, "unable to start manager")
		return err
	}

	// create CRDs
	err = crd.Create(ctx, mgr.GetConfig())
	if err != nil {
		return fmt.Errorf("unable to create crds: %v", err)
	}

	var enabledControllers []controller
	var coreControllers = []controller{
		&ClusterReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
			Logger: s.logger.WithName("cluster-controller"),
		},
		&InventoryReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
			Logger: s.logger.WithName("inventory-controller"),
		},
		&ClusterEventReconciler{
			Client:        mgr.GetClient(),
			Scheme:        mgr.GetScheme(),
			Logger:        s.logger.WithName("cluster-event-controller"),
			EventRecorder: mgr.GetEventRecorderFor("seeder"),
		},
		rufiocontrollers.NewMachineReconciler(
			mgr.GetClient(),
			mgr.GetEventRecorderFor("machine-controller"),
			rufiocontrollers.NewClientFunc(defaultRufioTimeout),
		),
		rufiojobwrapper.NewRufioWrapper(ctx,
			mgr.GetClient(),
		),
		rufiocontrollers.NewTaskReconciler(
			mgr.GetClient(),
			rufiocontrollers.NewClientFunc(defaultRufioTimeout),
		),
	}

	// embed mode doesnt need inventory events as they eventually flow into cluster events
	var nonEmbedModeControllers = []controller{
		&AddressPoolReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
			Logger: s.logger.WithName("addresspool-controller"),
		},
		&InventoryEventReconciler{
			Client:        mgr.GetClient(),
			Scheme:        mgr.GetScheme(),
			Logger:        s.logger.WithName("inventory-event-controller"),
			EventRecorder: mgr.GetEventRecorderFor("seeder"),
		},
		&WorkflowReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
			Logger: s.logger.WithName("workflow-controller"),
		},
	}

	var embedModeControllers = []controller{
		&LocalClusterReconciler{
			Client:        mgr.GetClient(),
			Scheme:        mgr.GetScheme(),
			Logger:        s.logger.WithName("local-cluster-controller"),
			EventRecorder: mgr.GetEventRecorderFor("seeder"),
		},
	}

	if s.EmbeddedMode {
		enabledControllers = append(coreControllers, embedModeControllers...)
	} else {
		enabledControllers = append(coreControllers, nonEmbedModeControllers...)
	}

	for _, v := range enabledControllers {
		if err := v.SetupWithManager(mgr); err != nil {
			return fmt.Errorf("error starting controllers: %v", err)
		}
	}

	// need a tmp client as mgr.Client read caches are unavailable
	// until manager has been started
	if s.EmbeddedMode {
		s.logger.Info("setting up local cluster objects")
		tmpClient, err := client.New(mgr.GetConfig(), client.Options{
			Scheme: scheme,
		})
		if err != nil {
			return fmt.Errorf("error creating temp client for local cluster setup: %v", err)
		}

		err = util.SetupLocalCluster(ctx, tmpClient)
		if err != nil {
			return fmt.Errorf("error setting up local cluster: %v", err)
		}
	}

	//+kubebuilder:scaffold:builder
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to setup health check: %v", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to setup readiness check: %v", err)
	}

	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		s.logger.Info("starting manager")
		return mgr.Start(egCtx)
	})

	eg.Go(func() error {
		s.logger.Info("starting webhook")
		return webhook.SetupWebhookServer(egCtx, mgr, s.LeaderElectionNamespace)
	})

	return eg.Wait()
}

func (s *Server) initLogs() {
	s.logger = zap.New(zap.UseDevMode(s.Debug))
}
