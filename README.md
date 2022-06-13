# Seeder

Seeder is an opensource project leveraging CNCF projects such as [tinkerbell](https://github.com/tinkerbell) to seed Harvester to metal devices in your environment.

The project is currently under active development and things may break.


## Quickstart
To get started you need a k8s cluster, running on the same L2 segment as your baremetal devices.

The helm [chart](./charts) will install seeder and associated components to your k8s cluster.

Once installed, a simple yaml manifest as follows will get the cluster provisioning kick started

```
apiVersion: metal.harvesterhci.io/v1alpha1
kind: AddressPool
metadata:
  name: node-pool
  namespace: default
spec:
  cidr: "172.16.128.11/32"
  gateway: "172.16.128.1"
  netmask: "255.255.248.0"
---
apiVersion: metal.harvesterhci.io/v1alpha1
kind: AddressPool
metadata:
  name: node2-pool
  namespace: default
spec:
  cidr: "172.16.128.12/32"
  gateway: "172.16.128.1"
  netmask: "255.255.248.0"
---
apiVersion: metal.harvesterhci.io/v1alpha1
kind: AddressPool
metadata:
  name: vip-pool
  namespace: default
spec:
  cidr: "172.16.128.7/32"
  gateway: "172.16.128.1"
  netmask: "255.255.248.0"
---

apiVersion: v1
kind: Secret
metadata:
  name: node
  namespace: default
stringData:
  username: "ADMIN"
  password: "ADMIN"
---
apiVersion: metal.harvesterhci.io/v1alpha1
kind: Inventory
metadata:
  name: node
  namespace: default
spec:
  primaryDisk: "/dev/sda"
  managementInterfaceMacAddress: "0c:c4:7a:6b:84:20"
  baseboardSpec:
    connection:
      host: "172.16.1.53"
      port: 623
      insecureTLS: true
      authSecretRef:
        name: node
        namespace: default
---
apiVersion: v1
kind: Secret
metadata:
  name: node2
  namespace: default
stringData:
  username: "ADMIN"
  password: "ADMIN"
---
apiVersion: metal.harvesterhci.io/v1alpha1
kind: Inventory
metadata:
  name: node2
  namespace: default
spec:
  primaryDisk: "/dev/sda"
  managementInterfaceMacAddress: "0c:c4:7a:6b:80:d0"
  baseboardSpec:
    connection:
      host: "172.16.1.52"
      port: 623
      insecureTLS: true
      authSecretRef:
        name: node2
        namespace: default
---
apiVersion: metal.harvesterhci.io/v1alpha1
kind: Cluster
metadata:
  name: first
  namespace: default
spec:
  version: "v1.0.2"
  imageURL: "http://172.16.135.50:8080"
  clusterConfig:
    nameservers:
      - 172.16.128.1
  nodes:
    - inventoryReference:
        name: node
        namespace: default
      addressPoolReference:
        name: node-pool
        namespace: default
    - inventoryReference:
        name: node2
        namespace: default
      addressPoolReference:
        name: node2-pool
        namespace: default
  vipConfig:
    addressPoolReference:
      name: vip-pool
      namespace: default

```

## Components
Seeder orchestrates infrastructure via a series of k8s CRDs and associated controllers.

The core seeder controller introduces 3 new CRDs

### AddressPools
Address pools allow users to define CIDR ranges to assign IP's from the underlying network. In the background seeder will leverage `tinkerbell` to perform the actual DHCP allocation and PXE booting of the nodes.

However to avoid the use case of having `tinkerbell` manage the entire subnet, the seeder controller will perform address allocation from the DHCP pool, and create static address allocation for `tinkerbell`. 

This allows users to leverage disjointed CIDR ranges and allocate IPs for the underlying metal nodes in the harvester cluster.

A sample address pool manifest is as follows:

```
apiVersion: metal.harvesterhci.io/v1alpha1
kind: AddressPool
metadata:
  name: node-pool
  namespace: default
spec:
  cidr: "172.16.128.11/32"
  gateway: "172.16.128.1"
  netmask: "255.255.248.0"
```

### Inventory
Inventory is an abstraction for metal nodes. Seeder will take the inventory object, and create a `baseboardmanagement` object, which is managed by [rufio](https://github.com/tinkerbell/rufio). `rufio` in turn performs all the associated baseboard operations, including rebooting and powering off the nodes based on conditions on the Inventory.

The idea behind inventory is to allow metal nodes to be re-purposed. 

The `baseboardSpec` in Inventory object, references a k8s secret where the username/password for the bmc.

A sample inventory manifest is as follows:

```
apiVersion: v1
kind: Secret
metadata:
  name: node
  namespace: default
stringData:
  username: "ADMIN"
  password: "ADMIN"
---
apiVersion: metal.harvesterhci.io/v1alpha1
kind: Inventory
metadata:
  name: node
  namespace: default
spec:
  primaryDisk: "/dev/sda"
  managementInterfaceMacAddress: "0c:c4:7a:6b:84:20"
  baseboardSpec:
    connection:
      host: "172.16.1.53"
      port: 623
      insecureTLS: true
      authSecretRef:
        name: node
        namespace: default
```

### Cluster
A cluster is just abstraction for the actual Harvester cluster. The cluster spec, includes common Harvester config that needs to be applied to the Inventory nodes making up the cluster.

The cluster has a list of inventory and associated addresspool configs from which addresses must be allocated for the harvester nodes.

The cluster spec also needs an `AddressPool` for allocating the Harvester VIP.

The cluster controller, orchestrates the allocation of ip addresses for the nodes from the address pools, generating the associated tinkerbell hardware spec, and then working with inventory controller to trigger the reboot of nodes to trigger the PXE booting of Inventory nodes and subsequent configuration of Harvester.

A sample cluster spec is as follows:

```
apiVersion: metal.harvesterhci.io/v1alpha1
kind: Cluster
metadata:
  name: first
  namespace: default
spec:
  version: "v1.0.2"
  imageURL: "http://172.16.135.50:8080"
  clusterConfig:
    nameservers:
      - 172.16.128.1
  nodes:
    - inventoryReference:
        name: node
        namespace: default
      addressPoolReference:
        name: node-pool
        namespace: default
    - inventoryReference:
        name: node2
        namespace: default
      addressPoolReference:
        name: node2-pool
        namespace: default
  vipConfig:
    addressPoolReference:
      name: vip-pool
      namespace: default
```      