package controllers

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

var _ = Describe("cluster events test", func() {
	var i *seederv1alpha1.Inventory
	var c *seederv1alpha1.Cluster
	var p1, p2 *seederv1alpha1.AddressPool
	var s *corev1.Secret
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
				Name:      "cluster-event-test-nodes",
				Namespace: "default",
			},
			Spec: seederv1alpha1.AddressSpec{
				CIDR:    "127.0.0.1/8",
				Gateway: "127.0.0.1",
			},
		}

		i = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-event-test",
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
						AuthSecretRef: corev1.SecretReference{
							Name:      "cluster-event-test",
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
				Name:      "cluster-event-test",
				Namespace: "default",
			},
			StringData: map[string]string{
				"username": "root",
				"password": "calvin",
			},
		}

		c = &seederv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-event-test",
				Namespace: "default",
			},
			Spec: seederv1alpha1.ClusterSpec{
				HarvesterVersion: "harvester_1_0_2",
				Nodes: []seederv1alpha1.NodeConfig{
					{
						InventoryReference: seederv1alpha1.ObjectReference{
							Name:      "cluster-event-test",
							Namespace: "default",
						},
						AddressPoolReference: seederv1alpha1.ObjectReference{
							Name:      "cluster-event-test-nodes",
							Namespace: "default",
						},
					},
				},
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
					ConfigURL: "file:///testdata/config.yaml",
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
			i.Spec.BaseboardManagementSpec.Connection.Host = redfishAddress
			return k8sClient.Create(ctx, i)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, p2)
		}, "30s", "5s").ShouldNot(HaveOccurred())

	})

	Describe("run cluster event reconcile", func() {
		It("patch cluster endpoints for cluster-event-test", func() {
			Eventually(func() error {
				apObj := &seederv1alpha1.AddressPool{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: p2.Name, Namespace: p2.Namespace}, apObj)
				if err != nil {
					return err
				}
				if apObj.Status.Status != seederv1alpha1.PoolReady {
					return fmt.Errorf("waiting for address pool to be marked ready")
				}

				if apObj.Status.AddressAllocation == nil {
					apObj.Status.AddressAllocation = make(map[string]seederv1alpha1.ObjectReferenceWithKind)
				}

				apObj.Status.AddressAllocation[k3sNodeAddress] = seederv1alpha1.ObjectReferenceWithKind{
					ObjectReference: seederv1alpha1.ObjectReference{
						Name:      i.Name,
						Namespace: i.Namespace,
					},
					Kind: seederv1alpha1.KindInventory,
				}
				return k8sClient.Status().Update(ctx, apObj)
			}, "30s", "5s").ShouldNot(HaveOccurred())

			Eventually(func() error {
				iObj := &seederv1alpha1.Inventory{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: i.Name, Namespace: i.Namespace}, iObj)
				if err != nil {
					return err
				}
				if iObj.Status.Status != seederv1alpha1.InventoryReady {
					return fmt.Errorf("waiting for inventory to be marked ready")
				}
				// patch inventory address to match k3s node
				iObj.Status.Address = k3sNodeAddress
				return k8sClient.Status().Update(ctx, iObj)
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
			}, "60s", "5s").ShouldNot(HaveOccurred())

			// poll for cluster to be ready for and for nodes to be patched with inventory info
			Eventually(func() error {
				iObj := &seederv1alpha1.Inventory{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, iObj); err != nil {
					return err
				}
				iObj.Labels = map[string]string{
					seederv1alpha1.OverrideRedfishPortLabel: redfishPort,
				}
				return k8sClient.Update(ctx, iObj)

			}, "30s", "5s").ShouldNot(HaveOccurred())

			Eventually(func() error {
				cObj := &seederv1alpha1.Cluster{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, cObj)
				if err != nil {
					return err
				}

				if cObj.Status.Status != seederv1alpha1.ClusterRunning {
					return fmt.Errorf("expected cluster to running but current status is %s", cObj.Status.Status)
				}
				return nil
			}, "120s", "10s").ShouldNot(HaveOccurred())

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
			}, "120s", "5s").ShouldNot(HaveOccurred())
		})
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

	})

})
