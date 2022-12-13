package crd

import (
	"context"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/rancher/wrangler/pkg/crd"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	tinkv1alpha1 "github.com/tinkerbell/tink/pkg/apis/core/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

func List() []crd.CRD {
	return []crd.CRD{
		newCRD("bmc.tinkerbell.org", &rufio.Machine{}, func(c crd.CRD) crd.CRD {
			return c.
				WithColumn("Status", ".status.powerState")
		}, "v1alpha1"),
		newCRD("bmc.tinkerbell.org", &rufio.Task{}, func(c crd.CRD) crd.CRD {
			return c.
				WithColumn("Status", ".status.status").
				WithShortNames("t")
		}, "v1alpha1"),
		newCRD("bmc.tinkerbell.org", &rufio.Job{}, func(c crd.CRD) crd.CRD {
			return c.
				WithColumn("Status", ".status.status").
				WithShortNames("j")
		}, "v1alpha1"),
		newCRD("metal.harvesterhci.io", &seederv1alpha1.AddressPool{}, func(c crd.CRD) crd.CRD {
			return c.
				WithColumn("AddressPoolStatus", ".status.status").
				WithColumn("StartAddress", ".status.startAddress").
				WithColumn("LastAddress", ".status.lastAddress").
				WithColumn("Netmask", ".status.netmask")
		}, "v1alpha1"),
		newCRD("metal.harvesterhci.io", &seederv1alpha1.Inventory{}, func(c crd.CRD) crd.CRD {
			return c.
				WithColumn("InventoryStatus", ".status.status").
				WithColumn("GeneratedPassword", ".status.generatedPassword").
				WithColumn("AllocatedNodeAddress", ".status.pxeBootConfig.address")
		}, "v1alpha1"),
		newCRD("metal.harvesterhci.io", &seederv1alpha1.Cluster{}, func(c crd.CRD) crd.CRD {
			return c.
				WithColumn("ClusterStatus", ".status.status").
				WithColumn("ClusterToken", ".status.token").
				WithColumn("ClusterAddress", ".status.ClusterAddress")
		}, "v1alpha1"),
		newCRD("tinkerbell.org", &tinkv1alpha1.Hardware{}, func(c crd.CRD) crd.CRD {
			return c.
				WithColumn("HardwareStatus", ".status.state")
		}, "v1alpha1"),
		newCRD("bmc.tinkerbell.org", &rufio.Machine{}, func(c crd.CRD) crd.CRD {
			return c.WithColumn("CurrentPowerState", ".status.status")
		}, "v1alpha1"),
		newCRD("bmc.tinkerbell.org", &rufio.Job{}, func(c crd.CRD) crd.CRD {
			return c.WithColumn("JobCompletionTime", ".status.completionTime")
		}, "v1alpha1"),
		newCRD("bmc.tinkerbell.org", &rufio.Task{}, func(c crd.CRD) crd.CRD {
			return c.WithColumn("TaskCompletionTime", ".status.completionTime")
		}, "v1alpha1"),
	}
}

func Create(ctx context.Context, cfg *rest.Config) error {
	factory, err := crd.NewFactoryFromClient(cfg)
	if err != nil {
		return err
	}

	return factory.BatchCreateCRDs(ctx, List()...).BatchWait()
}

func newCRD(group string, obj interface{}, customize func(crd.CRD) crd.CRD, version string) crd.CRD {
	crd := crd.CRD{
		GVK: schema.GroupVersionKind{
			Group:   group,
			Version: version,
		},
		Status:       true,
		NonNamespace: false,
		SchemaObject: obj,
	}
	if customize != nil {
		crd = customize(crd)
	}
	return crd
}
