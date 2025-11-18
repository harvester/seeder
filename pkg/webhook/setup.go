package webhook

import (
	"context"
	"time"

	"github.com/harvester/webhook/pkg/config"
	"github.com/harvester/webhook/pkg/server"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

const (
	defaultThreadiness = 5
	webhookServerName  = "harvester-seeder"
	defaultListenPort  = 443
)

func SetupWebhookServer(ctx context.Context, mgr manager.Manager, namespace string) error {
	opts := &config.Options{
		Threadiness:     defaultThreadiness,
		HTTPSListenPort: defaultListenPort,
		Namespace:       namespace,
	}

	webhookServer := server.NewWebhookServer(ctx, mgr.GetConfig(), webhookServerName, opts)

	if err := webhookServer.RegisterValidators(NewClusterValidator(ctx, mgr)); err != nil {
		return err
	}

	if err := webhookServer.RegisterValidators(NewInventoryValidatory(ctx, mgr)); err != nil {
		return err
	}

	// since webhook and manager start run as two go routines, need to wait for caches to sync
	// before starting the server
	for {
		informer, err := mgr.GetCache().GetInformer(ctx, &seederv1alpha1.Inventory{})
		if err != nil {
			return err
		}

		if informer.HasSynced() {
			break
		}

		time.Sleep(5 * time.Second)
	}

	if err := webhookServer.Start(); err != nil {
		return err
	}

	<-ctx.Done()
	return nil
}
