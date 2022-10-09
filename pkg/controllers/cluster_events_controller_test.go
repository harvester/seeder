package controllers

import (
	"fmt"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("cluster events test", func() {
	var i *seederv1alpha1.Inventory
	var c *seederv1alpha1.Cluster
	var p1, p2 *seederv1alpha1.AddressPool
	var s *corev1.Secret
	var nodeMock *dockertest.Resource
	var address string
	BeforeEach(func() {

		p1 = &seederv1alpha1.AddressPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-event-test",
				Namespace: "default",
			},
			Spec: seederv1alpha1.AddressSpec{
				CIDR:    "127.0.0.1/8",
				Gateway: "127.0.0.1",
			},
		}

		// empty addresspool. We will launch a k3s node with cluster config
		// and use node address to populate address pool.
		// this will ensure that during the reconcile process the inventory address
		// will match the node address, allowing the node to inventory mapping to be performed
		p2 = &seederv1alpha1.AddressPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "inventory-event-test",
				Namespace: "default",
			},
			Spec: seederv1alpha1.AddressSpec{},
		}

		i = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "event-test",
				Namespace: "default",
			},
			Spec: seederv1alpha1.InventorySpec{
				PrimaryDisk:                   "/dev/sda",
				ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
				BaseboardManagementSpec: rufio.BaseboardManagementSpec{
					Connection: rufio.Connection{
						Host:        "localhost",
						Port:        623,
						InsecureTLS: true,
						AuthSecretRef: corev1.SecretReference{
							Name:      "event-test",
							Namespace: "default",
						},
					},
				},
				Events: seederv1alpha1.Events{
					Enabled: true,
				},
			},
		}

		s = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "event-test",
				Namespace: "default",
			},
			StringData: map[string]string{
				"username": "root",
				"password": "calvin",
			},
		}

		c = &seederv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "add-cluster",
				Namespace: "default",
			},
			Spec: seederv1alpha1.ClusterSpec{
				HarvesterVersion: "harvester_1_0_2",
				Nodes:            []seederv1alpha1.NodeConfig{}, // node config will be patched later
				VIPConfig: seederv1alpha1.VIPConfig{
					AddressPoolReference: seederv1alpha1.ObjectReference{
						Name:      "cluster-event-test",
						Namespace: "default",
					},
				},
				ClusterConfig: seederv1alpha1.ClusterConfig{
					SSHKeys: []string{
						"abc",
						"def",
					},
					ConfigURL: "localhost:30300/config.yaml",
				},
			},
		}

		Eventually(func() error {
			return k8sClient.Create(ctx, p1)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, s)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, c)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, i)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			cObj := &seederv1alpha1.Cluster{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, cObj)
			if err != nil {
				return err
			}

			if cObj.Status.ClusterToken == "" {
				return fmt.Errorf("waiting for cluster token to be generated")
			}

			k3sRunOpts := &dockertest.RunOptions{
				Name:       "k3s-mock",
				Repository: "rancher/k3s",
				Tag:        "v1.24.2-k3s1",
				Cmd:        []string{"server", "--cluster-init"},
				Env: []string{
					fmt.Sprintf("K3S_TOKEN=%s", cObj.Status.ClusterToken),
				},
				Mounts: []string{
					"tmpfs:/run",
					"tmpfs:/var/run",
				},
				Privileged: true,
				ExposedPorts: []string{
					"6443/tcp",
				},
			}

			nodeMock, err = pool.RunWithOptions(k3sRunOpts, func(config *docker.HostConfig) {
				// set AutoRemove to true so that stopped container goes away by itself
				config.RestartPolicy = docker.RestartPolicy{
					Name: "no",
				}
			})

			if err != nil {
				return err
			}

			// patch port on cluster labels
			if cObj.Labels == nil {
				cObj.Labels = make(map[string]string)
			}

			// since mock node is k3s, need to change prefix from rke2 to k3s
			seederv1alpha1.DefaultAPIPrefix = "k3s"
			cObj.Labels[seederv1alpha1.OverrideAPIPortLabel] = nodeMock.GetPort("6443/tcp")
			networks, err := pool.NetworksByName("bridge")
			if err != nil {
				return err
			}

			if len(networks) != 1 {
				return fmt.Errorf("expected to find exactly 1 bridge network but found %d", len(networks))
			}

			address = nodeMock.GetIPInNetwork(&networks[0])
			p2.Spec.CIDR = fmt.Sprintf("%s/32", address)
			p2.Spec.Gateway = networks[0].Network.IPAM.Config[0].Gateway

			fmt.Println(p2.Spec.CIDR)
			fmt.Println(p2.Spec.Gateway)
			err = k8sClient.Create(ctx, p2)
			if err != nil {
				return err
			}

			// patch inventory into cluster
			nodeConfig := []seederv1alpha1.NodeConfig{
				{
					InventoryReference: seederv1alpha1.ObjectReference{
						Name:      i.Name,
						Namespace: i.Namespace,
					},
					AddressPoolReference: seederv1alpha1.ObjectReference{
						Name:      p2.Name,
						Namespace: p2.Namespace,
					},
				},
			}

			// add nodes to inventory
			cObj.Spec.Nodes = nodeConfig
			return k8sClient.Update(ctx, cObj)
		}, "60s", "5s").ShouldNot(HaveOccurred())
	})

	It("check for cluster event reconcilliation", func() {
		// poll for cluster to be ready for and for nodes to be patched with inventory info
		Eventually(func() error {
			cObj := &seederv1alpha1.Cluster{}
			fmt.Println(cObj.Spec.Nodes)
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, cObj)
			if err != nil {
				return err
			}

			if cObj.Status.Status != seederv1alpha1.ClusterRunning {
				return fmt.Errorf("expected cluster to running but current status is %s", cObj.Status.Status)
			}
			return nil
		}, "60s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			cObj := &seederv1alpha1.Cluster{}
			fmt.Println(cObj.Spec.Nodes)
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, cObj)
			if err != nil {
				return err
			}
			remoteClient, err := genCoreTypedClient(ctx, cObj)
			if err != nil {
				return err
			}

			nodeList, err := remoteClient.Nodes().List(ctx, metav1.ListOptions{})
			if err != nil {
				return err
			}

			iObj := &seederv1alpha1.Inventory{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, iObj)
			if err != nil {
				return err
			}

			var found bool
			for _, v := range nodeList.Items {
				for _, a := range v.Status.Addresses {
					if a.Address == iObj.Status.Address {
						found = true
						if _, ok := v.Labels["manufacturer"]; !ok {
							return fmt.Errorf("waiting for manufacturer to be populated")
						}
					}
				}
			}

			if !found {
				return fmt.Errorf("waiting to find node matching ip address allocated to inventory %s", iObj.Status.Address)
			}
			return nil
		}, "60s", "5s").ShouldNot(HaveOccurred())
	})

	AfterEach(func() {

		Eventually(func() error {
			// check and delete cluster if needed. Need this since one of the tests simulates removing cluster
			// and checking gc of hardware objects
			cObj := &seederv1alpha1.Cluster{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, cObj)
			if err != nil {
				if apierrors.IsNotFound(err) {
					return nil
				} else {
					return err
				}
			}
			return k8sClient.Delete(ctx, c)
		}, "30s", "5s").ShouldNot(HaveOccurred())
		Eventually(func() error {
			return k8sClient.Delete(ctx, i)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, s)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, p1)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, p2)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			cObj := &seederv1alpha1.Cluster{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, cObj)
			if err != nil {
				if apierrors.IsNotFound(err) {
					return nil
				}
				return err
			}

			return fmt.Errorf("waiting for cluster finalizers to finish")
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return pool.Purge(nodeMock)
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

})
