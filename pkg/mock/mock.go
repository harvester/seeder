package mock

import (
	"strings"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/rancher/wrangler/pkg/yaml"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
---
apiVersion: metal.harvesterhci.io/v1alpha1
kind: Cluster
metadata:
  name: test-mock-cluster-running
  namespace: default
spec:
  clusterConfig:
    nameservers:
    - 8.8.8.8
  imageURL: http://localhost/iso/
  nodes:
  - addressPoolReference:
      name: mock-pool
      namespace: default
    inventoryReference:
      name: inventory-1
      namespace: default
  version: v1.1.0
  vipConfig:
    addressPoolReference:
      name: mock-pool
      namespace: default
status:
  clusterAddress: 127.0.0.1
  status: clusterRunning
  token: ZgSyOCtX4TowsNjP
---
apiVersion: metal.harvesterhci.io/v1alpha1
kind: Cluster
metadata:
  name: test-mock-cluster-not-running
  namespace: default
spec:
  clusterConfig:
    nameservers:
    - 8.8.8.8
  imageURL: http://localhost/iso/
  nodes:
  - addressPoolReference:
      name: mock-pool
      namespace: default
    inventoryReference:
      name: inventory-2
      namespace: default
  version: v1.1.0
  vipConfig:
    addressPoolReference:
      name: mock-pool
      namespace: default
status:
  clusterAddress: 127.0.0.1
  status: clusterConfigReady
  token: ZgSyOCtX4TowsNjP
---
apiVersion: metal.harvesterhci.io/v1alpha1
kind: Inventory
metadata:
  name: inventory-1
  namespace: default
spec:
  baseboardSpec:
    connection:
      authSecretRef:
        name: hp-ilo
        namespace: seeder
      host: localhost
      insecureTLS: true
      port: 623
  events:
    enabled: true
    pollingInterval: 1h
  managementInterfaceMacAddress: 5c:b9:01:89:c6:61
  primaryDisk: /dev/sda
status:
  ownerCluster:
    name: ""
    namespace: ""
  pxeBootConfig: {}
  status: inventoryNodeReady
---
apiVersion: metal.harvesterhci.io/v1alpha1
kind: Inventory
metadata:
  name: inventory-2
  namespace: default
spec:
  baseboardSpec:
    connection:
      authSecretRef:
        name: hp-ilo
        namespace: seeder
      host: localhost
      insecureTLS: true
      port: 623
  events:
    enabled: true
    pollingInterval: 1h
  managementInterfaceMacAddress: 5c:b9:01:89:c6:61
  primaryDisk: /dev/sda
status:
  ownerCluster:
    name: ""
    namespace: ""
  pxeBootConfig: {}
  status: ""
---
apiVersion: metal.harvesterhci.io/v1alpha1
kind: AddressPool
metadata:
  name: mock-pool
  namespace: default
spec:
  cidr: 127.0.0.1/24
  gateway: 127.0.0.1
  netmask: 255.255.255.0
status:
  availableAddresses: 255
  lastAddress: 127.0.0.255
  netmask: 255.255.255.0
  startAddress: 127.0.0.1
  status: poolReady
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
