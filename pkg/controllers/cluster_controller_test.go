package controllers

import (
	"fmt"

	bmaasv1alpha1 "github.com/harvester/bmaas/pkg/api/v1alpha1"
	"github.com/harvester/bmaas/pkg/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	tinkv1alpha1 "github.com/tinkerbell/tink/pkg/apis/core/v1alpha1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Create cluster tests", func() {
	var i *bmaasv1alpha1.Inventory
	var c *bmaasv1alpha1.Cluster
	var a *bmaasv1alpha1.AddressPool
	var creds *v1.Secret
	BeforeEach(func() {
		a = &bmaasv1alpha1.AddressPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-test",
				Namespace: "default",
			},
			Spec: bmaasv1alpha1.AddressSpec{
				CIDR:    "192.168.1.1/29",
				Gateway: "192.168.1.7",
			},
		}

		i = &bmaasv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-test",
				Namespace: "default",
			},
			Spec: bmaasv1alpha1.InventorySpec{
				PrimaryDisk:                   "/dev/sda",
				ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
				BaseboardManagementSpec: rufio.BaseboardManagementSpec{
					Connection: rufio.Connection{
						Host:        "localhost",
						Port:        623,
						InsecureTLS: true,
						AuthSecretRef: v1.SecretReference{
							Name:      "cluster-test",
							Namespace: "default",
						},
					},
				},
			},
		}

		creds = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-test",
				Namespace: "default",
			},
			StringData: map[string]string{
				"username": "admin",
				"password": "password",
			},
		}

		c = &bmaasv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-test",
				Namespace: "default",
			},
			Spec: bmaasv1alpha1.ClusterSpec{
				HarvesterVersion: "harvester_1_0_2",
				Nodes: []bmaasv1alpha1.NodeConfig{
					{
						InventoryReference: bmaasv1alpha1.ObjectReference{
							Name:      "cluster-test",
							Namespace: "default",
						},
						AddressPoolReference: bmaasv1alpha1.ObjectReference{
							Name:      "cluster-test",
							Namespace: "default",
						},
					},
				},
				VIPConfig: bmaasv1alpha1.VIPConfig{
					AddressPoolReference: bmaasv1alpha1.ObjectReference{
						Name:      "cluster-test",
						Namespace: "default",
					},
				},
				ClusterConfig: bmaasv1alpha1.ClusterConfig{
					SSHKeys: []string{
						"abc",
						"def",
					},
					ConfigURL: "localhost:30300/config.yaml",
				},
			},
		}

		Eventually(func() error {
			return k8sClient.Create(ctx, a)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, creds)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, i)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, c)
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})
	It("check address pool reconcile in cluster controller workflow", func() {

		Eventually(func() error {
			obj := &bmaasv1alpha1.AddressPool{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: a.Namespace, Name: a.Name}, obj)
			if err != nil {
				return err
			}

			if obj.Status.Status != bmaasv1alpha1.PoolReady {
				return fmt.Errorf("waiting for pool to be ready. current status is %s", obj.Status.Status)
			}
			return nil
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	It("check inventory reconcile in cluster controller workflow", func() {
		Eventually(func() error {
			tmpInventory := &bmaasv1alpha1.Inventory{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, tmpInventory)
			if err != nil {
				return err
			}

			// is inventory ready
			if tmpInventory.Status.Status != bmaasv1alpha1.InventoryReady {
				return fmt.Errorf("expected inventory to be ready, but current state is %v", tmpInventory)
			}

			if !util.ConditionExists(tmpInventory.Status.Conditions, bmaasv1alpha1.InventoryAllocatedToCluster) {
				return fmt.Errorf("expected inventory to be allocated to cluster %v", tmpInventory.Status)
			}
			// is tinkerbell workflow condition present
			if !util.ConditionExists(tmpInventory.Status.Conditions, bmaasv1alpha1.TinkWorkflowCreated) {
				return fmt.Errorf("expected tinkerbell hardware condition to exist %v", tmpInventory.Status.Conditions)
			}

			// is bmcjob completed
			if !util.ConditionExists(tmpInventory.Status.Conditions, bmaasv1alpha1.BMCJobComplete) {
				return fmt.Errorf("expected associated bmcjob completion condition to exist %v", tmpInventory.Status.Conditions)
			}
			return nil
		}, "60s", "5s").ShouldNot(HaveOccurred())
	})

	It("reconcile hardware workflow in cluster controller reconcile", func() {
		Eventually(func() error {
			tmpCluster := &bmaasv1alpha1.Cluster{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, tmpCluster); err != nil {
				return err
			}

			if tmpCluster.Status.Status != bmaasv1alpha1.ClusterTinkHardwareSubmitted {
				return fmt.Errorf("expected status to be tink hardware submitted")
			}

			hwList := &tinkv1alpha1.HardwareList{}
			if err := k8sClient.List(ctx, hwList); err != nil {
				return err
			}

			if len(hwList.Items) != 1 {
				return fmt.Errorf("exepcted to find 1 hardware object but found %d", len(hwList.Items))
			}

			return nil
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	// check cluster deletion and reconcilliation of hardware and inventory objects
	// Test is flaky when using TestEnv. Disabling for now
	It("delete cluster and check cleanup of inventory objects", func() {
		Eventually(func() error {
			return k8sClient.Delete(ctx, c)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			iObj := &bmaasv1alpha1.Inventory{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, iObj); err != nil {
				return err
			}

			if len(iObj.Status.Conditions) != 1 {
				return fmt.Errorf("expected 1 conditions but found %d conditions %v", len(iObj.Status.Conditions), iObj.Status)
			}

			return nil
		}, "60s", "5s").ShouldNot(HaveOccurred())
	})

	AfterEach(func() {

		Eventually(func() error {
			// check and delete cluster if needed. Need this since one of the tests simulates removing cluster
			// and checking gc of hardware objects
			cObj := &bmaasv1alpha1.Cluster{}
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
			return k8sClient.Delete(ctx, creds)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, a)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			cObj := &bmaasv1alpha1.Cluster{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, cObj)
			if err != nil {
				if apierrors.IsNotFound(err) {
					return nil
				}
				return err
			}

			return fmt.Errorf("waiting for cluster finalizers to finish")
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})
})

var _ = Describe("add inventory to cluster tests", func() {
	var i, i2 *bmaasv1alpha1.Inventory
	var c *bmaasv1alpha1.Cluster
	var a *bmaasv1alpha1.AddressPool
	var creds, creds2 *v1.Secret
	BeforeEach(func() {
		a = &bmaasv1alpha1.AddressPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "add-cluster-test",
				Namespace: "default",
			},
			Spec: bmaasv1alpha1.AddressSpec{
				CIDR:    "192.168.1.1/29",
				Gateway: "192.168.1.7",
			},
		}

		i = &bmaasv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "node1",
				Namespace: "default",
			},
			Spec: bmaasv1alpha1.InventorySpec{
				PrimaryDisk:                   "/dev/sda",
				ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
				BaseboardManagementSpec: rufio.BaseboardManagementSpec{
					Connection: rufio.Connection{
						Host:        "localhost",
						Port:        623,
						InsecureTLS: true,
						AuthSecretRef: v1.SecretReference{
							Name:      "node1",
							Namespace: "default",
						},
					},
				},
			},
		}

		i2 = &bmaasv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "node2",
				Namespace: "default",
			},
			Spec: bmaasv1alpha1.InventorySpec{
				PrimaryDisk:                   "/dev/sda",
				ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
				BaseboardManagementSpec: rufio.BaseboardManagementSpec{
					Connection: rufio.Connection{
						Host:        "localhost",
						Port:        623,
						InsecureTLS: true,
						AuthSecretRef: v1.SecretReference{
							Name:      "node2",
							Namespace: "default",
						},
					},
				},
			},
		}

		creds = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "node1",
				Namespace: "default",
			},
			StringData: map[string]string{
				"username": "admin",
				"password": "password",
			},
		}

		creds2 = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "node2",
				Namespace: "default",
			},
			StringData: map[string]string{
				"username": "admin",
				"password": "password",
			},
		}

		c = &bmaasv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "add-cluster",
				Namespace: "default",
			},
			Spec: bmaasv1alpha1.ClusterSpec{
				HarvesterVersion: "harvester_1_0_2",
				Nodes: []bmaasv1alpha1.NodeConfig{
					{
						InventoryReference: bmaasv1alpha1.ObjectReference{
							Name:      "node1",
							Namespace: "default",
						},
						AddressPoolReference: bmaasv1alpha1.ObjectReference{
							Name:      "add-cluster-test",
							Namespace: "default",
						},
					},
				},
				VIPConfig: bmaasv1alpha1.VIPConfig{
					AddressPoolReference: bmaasv1alpha1.ObjectReference{
						Name:      "add-cluster-test",
						Namespace: "default",
					},
				},
				ClusterConfig: bmaasv1alpha1.ClusterConfig{
					SSHKeys: []string{
						"abc",
						"def",
					},
					ConfigURL: "localhost:30300/config.yaml",
				},
			},
		}

		Eventually(func() error {
			return k8sClient.Create(ctx, a)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, creds)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, creds2)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, i)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, i2)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, c)
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	It("add inventory reconcile in cluster controller workflow", func() {
		// add a noed to a cluster
		Eventually(func() error {
			cObj := &bmaasv1alpha1.Cluster{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, cObj)
			if err != nil {
				return err
			}

			if cObj.Status.Status != bmaasv1alpha1.ClusterTinkHardwareSubmitted {
				return fmt.Errorf("waiting for cluster to complete initial reconcilliation")
			}

			cObj.Spec.Nodes = append(cObj.Spec.Nodes, bmaasv1alpha1.NodeConfig{
				InventoryReference: bmaasv1alpha1.ObjectReference{
					Name:      i2.Name,
					Namespace: i2.Namespace,
				},
				AddressPoolReference: bmaasv1alpha1.ObjectReference{
					Name:      a.Name,
					Namespace: a.Namespace,
				},
			})

			return k8sClient.Update(ctx, cObj)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		// reconcile status of additional node
		Eventually(func() error {
			iObj := &bmaasv1alpha1.Inventory{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i2.Namespace, Name: i2.Name}, iObj)
			if err != nil {
				return err
			}

			if util.ConditionExists(iObj.Status.Conditions, bmaasv1alpha1.InventoryAllocatedToCluster) {
				return nil
			}
			fmt.Println(iObj.Status.Conditions)
			return fmt.Errorf("waiting for inventory to be allocated to cluster")
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	AfterEach(func() {

		Eventually(func() error {
			// check and delete cluster if needed. Need this since one of the tests simulates removing cluster
			// and checking gc of hardware objects
			cObj := &bmaasv1alpha1.Cluster{}
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
			return k8sClient.Delete(ctx, i2)
		}, "30s", "5s").ShouldNot(HaveOccurred())
		Eventually(func() error {
			return k8sClient.Delete(ctx, creds)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, creds2)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, a)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			cObj := &bmaasv1alpha1.Cluster{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, cObj)
			if err != nil {
				if apierrors.IsNotFound(err) {
					return nil
				}
				return err
			}

			return fmt.Errorf("waiting for cluster finalizers to finish")
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})
})

var _ = Describe("delete inventory from cluster tests", func() {
	var i, i2 *bmaasv1alpha1.Inventory
	var c *bmaasv1alpha1.Cluster
	var a *bmaasv1alpha1.AddressPool
	var creds, creds2 *v1.Secret
	BeforeEach(func() {
		a = &bmaasv1alpha1.AddressPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "del-cluster-test",
				Namespace: "default",
			},
			Spec: bmaasv1alpha1.AddressSpec{
				CIDR:    "192.168.1.1/29",
				Gateway: "192.168.1.7",
			},
		}

		i = &bmaasv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "del-node1",
				Namespace: "default",
			},
			Spec: bmaasv1alpha1.InventorySpec{
				PrimaryDisk:                   "/dev/sda",
				ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
				BaseboardManagementSpec: rufio.BaseboardManagementSpec{
					Connection: rufio.Connection{
						Host:        "localhost",
						Port:        623,
						InsecureTLS: true,
						AuthSecretRef: v1.SecretReference{
							Name:      "del-node1",
							Namespace: "default",
						},
					},
				},
			},
		}

		i2 = &bmaasv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "del-node2",
				Namespace: "default",
			},
			Spec: bmaasv1alpha1.InventorySpec{
				PrimaryDisk:                   "/dev/sda",
				ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
				BaseboardManagementSpec: rufio.BaseboardManagementSpec{
					Connection: rufio.Connection{
						Host:        "localhost",
						Port:        623,
						InsecureTLS: true,
						AuthSecretRef: v1.SecretReference{
							Name:      "del-node2",
							Namespace: "default",
						},
					},
				},
			},
		}

		creds = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "del-node1",
				Namespace: "default",
			},
			StringData: map[string]string{
				"username": "admin",
				"password": "password",
			},
		}

		creds2 = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "del-node2",
				Namespace: "default",
			},
			StringData: map[string]string{
				"username": "admin",
				"password": "password",
			},
		}

		c = &bmaasv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "del-cluster",
				Namespace: "default",
			},
			Spec: bmaasv1alpha1.ClusterSpec{
				HarvesterVersion: "harvester_1_0_2",
				Nodes: []bmaasv1alpha1.NodeConfig{
					{
						InventoryReference: bmaasv1alpha1.ObjectReference{
							Name:      "del-node1",
							Namespace: "default",
						},
						AddressPoolReference: bmaasv1alpha1.ObjectReference{
							Name:      "del-cluster-test",
							Namespace: "default",
						},
					},
					{
						InventoryReference: bmaasv1alpha1.ObjectReference{
							Name:      "del-node2",
							Namespace: "default",
						},
						AddressPoolReference: bmaasv1alpha1.ObjectReference{
							Name:      "del-cluster-test",
							Namespace: "default",
						},
					},
				},
				VIPConfig: bmaasv1alpha1.VIPConfig{
					AddressPoolReference: bmaasv1alpha1.ObjectReference{
						Name:      "del-cluster-test",
						Namespace: "default",
					},
				},
				ClusterConfig: bmaasv1alpha1.ClusterConfig{
					SSHKeys: []string{
						"abc",
						"def",
					},
					ConfigURL: "localhost:30300/config.yaml",
				},
			},
		}

		Eventually(func() error {
			return k8sClient.Create(ctx, a)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, creds)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, creds2)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, i)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, i2)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, c)
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	It("remove inventory reconcile in cluster controller workflow", func() {
		// add a noed to a cluster
		Eventually(func() error {
			cObj := &bmaasv1alpha1.Cluster{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, cObj)
			if err != nil {
				return err
			}

			if cObj.Status.Status != bmaasv1alpha1.ClusterTinkHardwareSubmitted {
				return fmt.Errorf("waiting for cluster to complete initial reconcilliation")
			}

			// remove the second node
			cObj.Spec.Nodes = []bmaasv1alpha1.NodeConfig{
				{
					InventoryReference: bmaasv1alpha1.ObjectReference{
						Name:      "del-node1",
						Namespace: "default",
					},
					AddressPoolReference: bmaasv1alpha1.ObjectReference{
						Name:      "del-cluster-test",
						Namespace: "default",
					},
				},
			}

			return k8sClient.Update(ctx, cObj)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		// reconcile status of additional node
		Eventually(func() error {
			iObj := &bmaasv1alpha1.Inventory{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i2.Namespace, Name: i2.Name}, iObj)
			if err != nil {
				return err
			}

			if !util.ConditionExists(iObj.Status.Conditions, bmaasv1alpha1.InventoryFreed) || util.ConditionExists(iObj.Status.Conditions, bmaasv1alpha1.InventoryAllocatedToCluster) {
				return nil
			}
			return fmt.Errorf("waiting for inventory to be freed")
		}, "60s", "5s").ShouldNot(HaveOccurred())

		// wait for hardware to be removed
		Eventually(func() error {
			hw := &tinkv1alpha1.Hardware{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i2.Namespace, Name: i2.Name}, hw)
			if err != nil {
				// object cleaned up
				if apierrors.IsNotFound(err) {
					return nil
				} else {
					return err
				}
			}

			return fmt.Errorf("waiting for hardware to be cleaned up")
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	AfterEach(func() {

		Eventually(func() error {
			// check and delete cluster if needed. Need this since one of the tests simulates removing cluster
			// and checking gc of hardware objects
			cObj := &bmaasv1alpha1.Cluster{}
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
			return k8sClient.Delete(ctx, i2)
		}, "30s", "5s").ShouldNot(HaveOccurred())
		Eventually(func() error {
			return k8sClient.Delete(ctx, creds)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, creds2)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, a)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			cObj := &bmaasv1alpha1.Cluster{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, cObj)
			if err != nil {
				if apierrors.IsNotFound(err) {
					return nil
				}
				return err
			}

			return fmt.Errorf("waiting for cluster finalizers to finish")
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})
})
