# seeder-cli
seeder plugin allows automation of some routine tasks with seeder.
It can be used as a kubectl plugin by placing it in your path, and renaming the binary as kubectl-seeder or a standalone binary

Currently supported sub commands are:
* gen-kubeconfig: will generate an admin kubeconfig for a harvester cluster provisioned via seeder
* create-cluster: will create a new cluster object with some basic options
* recreate-cluster: will delete and re-create the cluster and patch the version if one is supplied

## gen-kubeconfig
```shell
Usage:
  seeder gen-kubeconfig $CLUSTER_NAME [flags]

Flags:
  -h, --help          help for gen-kubeconfig
  -p, --path string   path to place generated harvester cluster kubeconfig

Global Flags:
  -d, --debug              enable debug logging
  -n, --namespace string   namespace
```

## create-cluster
```shell
Usage:
  seeder create-cluster $CLUSTER_NAME [options] [flags]

Flags:
      --address-pool string   addresspool to be used for address allocation for VIP and inventory nodes
      --config-url string     [optional] location of common harvester config that will be applied to all nodes
  -h, --help                  help for create-cluster
      --image-url string      [optional] location where artifacts for pxe booting inventory are present
      --inventory strings     list of inventory objects in namespace to be used for cluster
      --static-vip string     [optional] static address for harvester cluster vip (optional). If not specified an address from addresspool will be used
  -v, --version string        version of harvester

Global Flags:
  -d, --debug              enable debug logging
  -n, --namespace string   namespace
```

## recreate-cluster
```sbell
Usage:
  seeder recreate-cluster $CLUSTER_NAME [flags]

Flags:
  -h, --help             help for recreate-cluster
  -v, --version string   [optional] version to use to recreate cluster

Global Flags:
  -d, --debug              enable debug logging
  -n, --namespace string   namespace
```