package controllers

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/util"
)

var _ = Describe("Create cluster tests", func() {
	var i *seederv1alpha1.Inventory
	var c *seederv1alpha1.Cluster
	var a *seederv1alpha1.AddressPool
	var creds *v1.Secret
	BeforeEach(func() {
		a = &seederv1alpha1.AddressPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-test",
				Namespace: "default",
			},
			Spec: seederv1alpha1.AddressSpec{
				CIDR:    "192.168.1.1/29",
				Gateway: "192.168.1.7",
			},
		}

		i = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-test",
				Namespace: "default",
			},
			Spec: seederv1alpha1.InventorySpec{
				PrimaryDisk:                   "/dev/sda",
				ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
				BaseboardManagementSpec: rufio.MachineSpec{
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

		c = &seederv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-test",
				Namespace: "default",
			},
			Spec: seederv1alpha1.ClusterSpec{
				HarvesterVersion: "harvester_1_0_2",
				Nodes: []seederv1alpha1.NodeConfig{
					{
						InventoryReference: seederv1alpha1.ObjectReference{
							Name:      "cluster-test",
							Namespace: "default",
						},
						AddressPoolReference: seederv1alpha1.ObjectReference{
							Name:      "cluster-test",
							Namespace: "default",
						},
					},
				},
				VIPConfig: seederv1alpha1.VIPConfig{
					AddressPoolReference: seederv1alpha1.ObjectReference{
						Name:      "cluster-test",
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
			obj := &seederv1alpha1.AddressPool{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: a.Namespace, Name: a.Name}, obj)
			if err != nil {
				return err
			}

			if obj.Status.Status != seederv1alpha1.PoolReady {
				return fmt.Errorf("waiting for pool to be ready. current status is %s", obj.Status.Status)
			}
			return nil
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	It("check inventory reconcile in cluster controller workflow", func() {
		Eventually(func() error {
			tmpInventory := &seederv1alpha1.Inventory{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, tmpInventory)
			if err != nil {
				return err
			}

			// is inventory ready
			if tmpInventory.Status.Status != seederv1alpha1.InventoryReady {
				return fmt.Errorf("expected inventory to be ready, but current state is %v", tmpInventory)
			}

			if !util.ConditionExists(tmpInventory, seederv1alpha1.InventoryAllocatedToCluster) {
				return fmt.Errorf("expected inventory to be allocated to cluster %v", tmpInventory.Status)
			}
			// is tinkerbell workflow condition present
			if !util.ConditionExists(tmpInventory, seederv1alpha1.TinkHardwareCreated) {
				return fmt.Errorf("expected tinkerbell hardware condition to exist %v", tmpInventory.Status.Conditions)
			}

			if tmpInventory.Status.PowerAction.LastActionStatus != seederv1alpha1.NodeJobComplete {
				return fmt.Errorf("expected power action to be completed but got %s", tmpInventory.Status.PowerAction.LastActionStatus)
			}
			return nil
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	It("reconcile hardware workflow in cluster controller reconcile", func() {
		Eventually(func() error {
			tmpCluster := &seederv1alpha1.Cluster{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, tmpCluster); err != nil {
				return err
			}

			if tmpCluster.Status.Status != seederv1alpha1.ClusterTinkHardwareSubmitted {
				return fmt.Errorf("expected status to be tink hardware submitted")
			}

			hwList := &tinkv1alpha1.HardwareList{}
			if err := k8sClient.List(ctx, hwList, &client.ListOptions{Namespace: "default"}); err != nil {
				return err
			}

			for _, v := range hwList.Items {
				if v.Name == i.Name && v.Namespace == i.Namespace {
					return nil
				}
			}

			return fmt.Errorf("did not find hardware matching the inventory %s", i.Name)
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	It("reconcile tinkerbell workflow in cluster controller reconcile", func() {
		Eventually(func() error {
			tmpCluster := &seederv1alpha1.Cluster{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, tmpCluster); err != nil {
				return err
			}

			if tmpCluster.Status.Status != seederv1alpha1.ClusterTinkHardwareSubmitted {
				return fmt.Errorf("expected status to be tink hardware submitted")
			}

			workflowList := &tinkv1alpha1.WorkflowList{}
			if err := k8sClient.List(ctx, workflowList, &client.ListOptions{Namespace: "default"}); err != nil {
				return err
			}

			for _, v := range workflowList.Items {
				if v.Name == i.Name && v.Namespace == i.Namespace {
					return nil
				}
			}

			return fmt.Errorf("did not find workflow matching the inventory %s", i.Name)
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})
	// check cluster deletion and reconcilliation of hardware and inventory objects
	// Test is flaky when using TestEnv. Disabling for now
	It("delete cluster and check cleanup of inventory objects", func() {
		Eventually(func() error {
			return k8sClient.Delete(ctx, c)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			iObj := &seederv1alpha1.Inventory{}
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
			return k8sClient.Delete(ctx, creds)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, a)
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
	})
})

var _ = Describe("add inventory to cluster tests", func() {
	var i, i2 *seederv1alpha1.Inventory
	var c *seederv1alpha1.Cluster
	var a *seederv1alpha1.AddressPool
	var creds, creds2 *v1.Secret
	BeforeEach(func() {
		a = &seederv1alpha1.AddressPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "add-cluster-test",
				Namespace: "default",
			},
			Spec: seederv1alpha1.AddressSpec{
				CIDR:    "192.168.1.1/29",
				Gateway: "192.168.1.7",
			},
		}

		i = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "node1",
				Namespace: "default",
			},
			Spec: seederv1alpha1.InventorySpec{
				PrimaryDisk:                   "/dev/sda",
				ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
				BaseboardManagementSpec: rufio.MachineSpec{
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

		i2 = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "node2",
				Namespace: "default",
			},
			Spec: seederv1alpha1.InventorySpec{
				PrimaryDisk:                   "/dev/sda",
				ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
				BaseboardManagementSpec: rufio.MachineSpec{
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

		c = &seederv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "add-cluster",
				Namespace: "default",
			},
			Spec: seederv1alpha1.ClusterSpec{
				HarvesterVersion: "harvester_1_0_2",
				Nodes: []seederv1alpha1.NodeConfig{
					{
						InventoryReference: seederv1alpha1.ObjectReference{
							Name:      "node1",
							Namespace: "default",
						},
						AddressPoolReference: seederv1alpha1.ObjectReference{
							Name:      "add-cluster-test",
							Namespace: "default",
						},
					},
				},
				VIPConfig: seederv1alpha1.VIPConfig{
					AddressPoolReference: seederv1alpha1.ObjectReference{
						Name:      "add-cluster-test",
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
			cObj := &seederv1alpha1.Cluster{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, cObj)
			if err != nil {
				return err
			}

			if cObj.Status.Status != seederv1alpha1.ClusterTinkHardwareSubmitted {
				return fmt.Errorf("waiting for cluster to complete initial reconcilliation")
			}

			cObj.Spec.Nodes = append(cObj.Spec.Nodes, seederv1alpha1.NodeConfig{
				InventoryReference: seederv1alpha1.ObjectReference{
					Name:      i2.Name,
					Namespace: i2.Namespace,
				},
				AddressPoolReference: seederv1alpha1.ObjectReference{
					Name:      a.Name,
					Namespace: a.Namespace,
				},
			})

			return k8sClient.Update(ctx, cObj)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		// reconcile status of additional node
		Eventually(func() error {
			iObj := &seederv1alpha1.Inventory{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i2.Namespace, Name: i2.Name}, iObj)
			if err != nil {
				return err
			}

			if util.ConditionExists(iObj, seederv1alpha1.InventoryAllocatedToCluster) {
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
	})
})

var _ = Describe("delete inventory from cluster tests", func() {
	var i, i2 *seederv1alpha1.Inventory
	var c *seederv1alpha1.Cluster
	var a *seederv1alpha1.AddressPool
	var creds, creds2 *v1.Secret
	BeforeEach(func() {
		a = &seederv1alpha1.AddressPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "del-cluster-test",
				Namespace: "default",
			},
			Spec: seederv1alpha1.AddressSpec{
				CIDR:    "192.168.1.1/29",
				Gateway: "192.168.1.7",
			},
		}

		i = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "del-node1",
				Namespace: "default",
			},
			Spec: seederv1alpha1.InventorySpec{
				PrimaryDisk:                   "/dev/sda",
				ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
				BaseboardManagementSpec: rufio.MachineSpec{
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

		i2 = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "del-node2",
				Namespace: "default",
			},
			Spec: seederv1alpha1.InventorySpec{
				PrimaryDisk:                   "/dev/sda",
				ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
				BaseboardManagementSpec: rufio.MachineSpec{
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

		c = &seederv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "del-cluster",
				Namespace: "default",
			},
			Spec: seederv1alpha1.ClusterSpec{
				HarvesterVersion: "harvester_1_0_2",
				Nodes: []seederv1alpha1.NodeConfig{
					{
						InventoryReference: seederv1alpha1.ObjectReference{
							Name:      "del-node1",
							Namespace: "default",
						},
						AddressPoolReference: seederv1alpha1.ObjectReference{
							Name:      "del-cluster-test",
							Namespace: "default",
						},
					},
					{
						InventoryReference: seederv1alpha1.ObjectReference{
							Name:      "del-node2",
							Namespace: "default",
						},
						AddressPoolReference: seederv1alpha1.ObjectReference{
							Name:      "del-cluster-test",
							Namespace: "default",
						},
					},
				},
				VIPConfig: seederv1alpha1.VIPConfig{
					AddressPoolReference: seederv1alpha1.ObjectReference{
						Name:      "del-cluster-test",
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
			cObj := &seederv1alpha1.Cluster{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, cObj)
			if err != nil {
				return err
			}

			if cObj.Status.Status != seederv1alpha1.ClusterTinkHardwareSubmitted {
				return fmt.Errorf("waiting for cluster to complete initial reconcilliation")
			}

			// remove the second node
			cObj.Spec.Nodes = []seederv1alpha1.NodeConfig{
				{
					InventoryReference: seederv1alpha1.ObjectReference{
						Name:      "del-node1",
						Namespace: "default",
					},
					AddressPoolReference: seederv1alpha1.ObjectReference{
						Name:      "del-cluster-test",
						Namespace: "default",
					},
				},
			}

			return k8sClient.Update(ctx, cObj)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		// reconcile status of additional node
		Eventually(func() error {
			iObj := &seederv1alpha1.Inventory{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i2.Namespace, Name: i2.Name}, iObj)
			if err != nil {
				return err
			}

			if !util.ConditionExists(iObj, seederv1alpha1.InventoryFreed) || util.ConditionExists(iObj, seederv1alpha1.InventoryAllocatedToCluster) {
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
	})
})

// this test attempts to check the last part of the workflow which generates a kubeconfig
// and checks the nodes on the cluster.
var _ = Describe("cluster running test", func() {
	var i *seederv1alpha1.Inventory
	var c *seederv1alpha1.Cluster
	var a *seederv1alpha1.AddressPool
	var creds *v1.Secret

	BeforeEach(func() {
		a = &seederv1alpha1.AddressPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kc-test",
				Namespace: "default",
			},
			Spec: seederv1alpha1.AddressSpec{
				CIDR:    "127.0.0.1/8",
				Gateway: "127.0.0.1",
			},
		}

		i = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kc-test",
				Namespace: "default",
			},
			Spec: seederv1alpha1.InventorySpec{
				PrimaryDisk:                   "/dev/sda",
				ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
				BaseboardManagementSpec: rufio.MachineSpec{
					Connection: rufio.Connection{
						Host:        "localhost",
						Port:        623,
						InsecureTLS: true,
						AuthSecretRef: v1.SecretReference{
							Name:      "kc-test",
							Namespace: "default",
						},
					},
				},
			},
		}

		creds = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kc-test",
				Namespace: "default",
			},
			StringData: map[string]string{
				"username": "root",
				"password": "calvin",
			},
		}

		c = &seederv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kc-test",
				Namespace: "default",
			},
			Spec: seederv1alpha1.ClusterSpec{
				HarvesterVersion: "harvester_1_0_2",
				Nodes: []seederv1alpha1.NodeConfig{
					{
						InventoryReference: seederv1alpha1.ObjectReference{
							Name:      "kc-test",
							Namespace: "default",
						},
						AddressPoolReference: seederv1alpha1.ObjectReference{
							Name:      "kc-test",
							Namespace: "default",
						},
					},
				},
				VIPConfig: seederv1alpha1.VIPConfig{
					AddressPoolReference: seederv1alpha1.ObjectReference{
						Name:      "kc-test",
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

		//Wait for token to be populated and then use the same to create
		//a k3s mock to validate kubeconfig
		Eventually(func() error {
			cObj := &seederv1alpha1.Cluster{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, cObj)
			if err != nil {
				return err
			}

			if cObj.Status.ClusterToken == "" {
				return fmt.Errorf("waiting for cluster token to be generated")
			}

			if cObj.Status.ClusterToken != defaultToken || cObj.Status.ClusterAddress != k3sNodeAddress {
				cObj.Status.ClusterToken = defaultToken
				cObj.Status.ClusterAddress = k3sNodeAddress
				if err := k8sClient.Status().Update(ctx, cObj); err != nil {
					return fmt.Errorf("error updating cluster token: %v", err)
				}
			}

			// patch port on cluster labels
			if cObj.Labels == nil {
				cObj.Labels = make(map[string]string)
			}

			// since mock node is k3s, need to change prefix from rke2 to k3s
			seederv1alpha1.DefaultAPIPrefix = "k3s"
			cObj.Labels[seederv1alpha1.OverrideAPIPortLabel] = k3sPort

			return k8sClient.Update(ctx, cObj)

		}, "30s", "5s").ShouldNot(HaveOccurred())
	})
	It("check if cluster is running", Label("skip-in-drone"), func() {

		Eventually(func() error {
			obj := &seederv1alpha1.Cluster{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, obj)
			if err != nil {
				return err
			}

			if obj.Status.Status != seederv1alpha1.ClusterRunning {
				return fmt.Errorf("waiting for cluster to be running. current status is %s", obj.Status.Status)
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
			return k8sClient.Delete(ctx, creds)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, a)
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
	})
})

// multi-node cluster test
var _ = Describe("multi-node cluster provisioning test", func() {
	var i, i2, i3 *seederv1alpha1.Inventory
	var c *seederv1alpha1.Cluster
	var a *seederv1alpha1.AddressPool
	var creds *v1.Secret
	BeforeEach(func() {
		a = &seederv1alpha1.AddressPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multi-node-test",
				Namespace: "default",
			},
			Spec: seederv1alpha1.AddressSpec{
				CIDR:    "192.168.1.1/29",
				Gateway: "192.168.1.7",
			},
		}

		i = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multi-node1",
				Namespace: "default",
			},
			Spec: seederv1alpha1.InventorySpec{
				PrimaryDisk:                   "/dev/sda",
				ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
				BaseboardManagementSpec: rufio.MachineSpec{
					Connection: rufio.Connection{
						Host:        "localhost",
						Port:        623,
						InsecureTLS: true,
						AuthSecretRef: v1.SecretReference{
							Name:      "common",
							Namespace: "default",
						},
					},
				},
			},
		}

		i2 = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multi-node2",
				Namespace: "default",
			},
			Spec: seederv1alpha1.InventorySpec{
				PrimaryDisk:                   "/dev/sda",
				ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
				BaseboardManagementSpec: rufio.MachineSpec{
					Connection: rufio.Connection{
						Host:        "localhost",
						Port:        623,
						InsecureTLS: true,
						AuthSecretRef: v1.SecretReference{
							Name:      "common",
							Namespace: "default",
						},
					},
				},
			},
		}

		i3 = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multi-node3",
				Namespace: "default",
			},
			Spec: seederv1alpha1.InventorySpec{
				PrimaryDisk:                   "/dev/sda",
				ManagementInterfaceMacAddress: "xx:xx:xx:xx:xx",
				BaseboardManagementSpec: rufio.MachineSpec{
					Connection: rufio.Connection{
						Host:        "localhost",
						Port:        623,
						InsecureTLS: true,
						AuthSecretRef: v1.SecretReference{
							Name:      "common",
							Namespace: "default",
						},
					},
				},
			},
		}

		creds = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "common",
				Namespace: "default",
			},
			StringData: map[string]string{
				"username": "admin",
				"password": "password",
			},
		}

		c = &seederv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multi-node-cluster",
				Namespace: "default",
			},
			Spec: seederv1alpha1.ClusterSpec{
				HarvesterVersion: "harvester_1_0_2",
				Nodes: []seederv1alpha1.NodeConfig{
					{
						InventoryReference: seederv1alpha1.ObjectReference{
							Name:      "multi-node1",
							Namespace: "default",
						},
						AddressPoolReference: seederv1alpha1.ObjectReference{
							Name:      "multi-node-test",
							Namespace: "default",
						},
					},
					{
						InventoryReference: seederv1alpha1.ObjectReference{
							Name:      "multi-node2",
							Namespace: "default",
						},
						AddressPoolReference: seederv1alpha1.ObjectReference{
							Name:      "multi-node-test",
							Namespace: "default",
						},
					},
					{
						InventoryReference: seederv1alpha1.ObjectReference{
							Name:      "multi-node3",
							Namespace: "default",
						},
						AddressPoolReference: seederv1alpha1.ObjectReference{
							Name:      "multi-node-test",
							Namespace: "default",
						},
					},
				},
				VIPConfig: seederv1alpha1.VIPConfig{
					AddressPoolReference: seederv1alpha1.ObjectReference{
						Name:      "multi-node-test",
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
			return k8sClient.Create(ctx, a)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, creds)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, i)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, i2)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, i3)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, c)
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	It("inventory reconcile in cluster controller workflow", func() {
		addMap := make(map[string]string)
		Eventually(func() error {
			cObj := &seederv1alpha1.Cluster{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, cObj)
			if err != nil {
				return err
			}
			if cObj.Status.Status != seederv1alpha1.ClusterTinkHardwareSubmitted {
				return fmt.Errorf("expected to find cluster status %s, but got %s", seederv1alpha1.ClusterTinkHardwareSubmitted, cObj.Status.Status)
			}
			return nil
		}, "30s", "5s").ShouldNot(HaveOccurred())

		invList := []*seederv1alpha1.Inventory{i, i2, i3}
		for _, v := range invList {
			Eventually(func() error {
				iObj := &seederv1alpha1.Inventory{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: v.Namespace, Name: v.Name}, iObj)
				if err != nil {
					return fmt.Errorf("error fetching inventory %s: %v", v.Name, err)
				}
				if nodeName, ok := addMap[iObj.Status.Address]; !ok {
					addMap[iObj.Status.Address] = iObj.Name
				} else {
					return fmt.Errorf("duplicate address: %s assigned to %s is already allocated to %s", iObj.Status.Address, iObj.Name, nodeName)
				}

				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		}

	})

	AfterEach(func() {
		Eventually(func() error {
			return k8sClient.Delete(ctx, c)
		}, "30s", "5s").ShouldNot(HaveOccurred())
		Eventually(func() error {
			return k8sClient.Delete(ctx, i)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, i2)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, i3)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, creds)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, a)
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
	})
})
