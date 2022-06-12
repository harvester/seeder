package mock

import (
	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/rancher/wrangler/pkg/yaml"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"strings"
)

const (
	DefaultObjects = `
apiVersion: bmc.tinkerbell.org/v1alpha1
kind: BaseboardManagement
metadata:
  name: fiftytwo
spec:
  connection:
    host: 172.16.1.52
    port: 623
    authSecretRef:
      name: fiftytwo
      namespace: default
    insecureTLS: false
  power: "on"
---
apiVersion: bmc.tinkerbell.org/v1alpha1
kind: BaseboardManagement
metadata:
  name: fiftythree
spec:
  connection:
    host: 172.16.1.52
    port: 623
    authSecretRef:
      name: fiftythree
      namespace: default
    insecureTLS: false
  power: "on"
---
apiVersion: v1
kind: Secret
metadata:
  name: fiftytwo
  namespace: default
stringData:
  "username": "ADMIN"
  "password": "ADMIN"
---
apiVersion: v1
kind: Secret
metadata:
  name: fiftyone
  namespace: default
stringData:
  "username": "ADMIN"
  "password": "ADMIN"
`
)

func generateObjects() ([]runtime.Object, error) {
	objs, err := yaml.ToObjects(strings.NewReader(DefaultObjects))
	return objs, err
}

// GenerateFakeClient is fake client with some preloaded objects to simplify unit tests
func GenerateFakeClient() (client.WithWatch, error) {
	objs, err := generateObjects()
	if err != nil {
		return nil, err
	}
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(seederv1alpha1.AddToScheme(scheme))
	utilruntime.Must(rufio.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
	return c, nil
}
