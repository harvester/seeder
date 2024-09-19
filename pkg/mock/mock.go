package mock

import (
	"strings"

	"github.com/rancher/wrangler/pkg/yaml"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

const (
	DefaultObjects = `
apiVersion: bmc.tinkerbell.org/v1alpha1
kind: Machine
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
kind: Machine
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
---
apiVersion: v1
kind: Namespace
metadata:
  name: "harvester-system"
`
)

var statusSubResources = []client.Object{&seederv1alpha1.Cluster{}, &seederv1alpha1.Inventory{}, &seederv1alpha1.AddressPool{}, &seederv1alpha1.Cluster{}}

func generateObjects() ([]runtime.Object, error) {
	objs, err := GenerateObjectsFromVar(DefaultObjects)
	return objs, err
}

func GenerateObjectsFromVar(v string) ([]runtime.Object, error) {
	objs, err := yaml.ToObjects(strings.NewReader(v))
	return objs, err
}

// GenerateFakeClient is fake client with some preloaded objects to simplify unit tests
func GenerateFakeClient() (client.WithWatch, error) {
	objs, err := generateObjects()
	if err != nil {
		return nil, err
	}

	return GenerateFakeClientFromObjects(objs)
}

func GenerateFakeClientFromObjects(objs []runtime.Object) (client.WithWatch, error) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(seederv1alpha1.AddToScheme(scheme))
	utilruntime.Must(rufio.AddToScheme(scheme))
	utilruntime.Must(tinkv1alpha1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).WithStatusSubresource(statusSubResources...).Build()
	return c, nil
}
