package crd

import (
	"bytes"
	"context"
	"fmt"

	"github.com/rancher/wrangler/pkg/apply"
	"github.com/rancher/wrangler/pkg/yaml"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"

	"github.com/harvester/seeder/pkg/data"
)

func Create(ctx context.Context, cfg *rest.Config) error {
	applyClient, err := apply.NewForConfig(cfg)
	if err != nil {
		return err
	}
	objs, err := generateObjects()
	if err != nil {
		return fmt.Errorf("error generating objects: %v", err)
	}

	return applyClient.WithDynamicLookup().WithContext(ctx).WithSetID("seeder-crd").ApplyObjects(objs...)
}

func generateObjects() ([]runtime.Object, error) {
	var objs []runtime.Object
	for _, v := range data.AssetNames() {
		content, err := data.Asset(v)
		if err != nil {
			return nil, err
		}
		obj, err := yaml.ToObjects(bytes.NewReader(content))
		if err != nil {
			return nil, err
		}
		objs = append(objs, obj...)
	}

	return objs, nil
}
