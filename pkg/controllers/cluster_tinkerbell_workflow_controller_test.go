package controllers

import (
	"fmt"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("reconcile tinkerbell workflow deletion test", func() {
	var i *seederv1alpha1.Inventory
	var c *seederv1alpha1.Cluster
	var a *seederv1alpha1.AddressPool
	var creds *v1.Secret
	BeforeEach(func() {
		a = &seederv1alpha1.AddressPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-workflow-test",
				Namespace: "default",
			},
			Spec: seederv1alpha1.AddressSpec{
				CIDR:    "192.168.1.1/29",
				Gateway: "192.168.1.7",
			},
		}

		i = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-workflow-test",
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
							Name:      "cluster-workflow-test",
							Namespace: "default",
						},
					},
				},
			},
		}

		creds = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-workflow-test",
				Namespace: "default",
			},
			StringData: map[string]string{
				"username": "admin",
				"password": "password",
			},
		}

		c = &seederv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-workflow-test",
				Namespace: "default",
			},
			Spec: seederv1alpha1.ClusterSpec{
				HarvesterVersion: "harvester_1_0_2",
				Nodes: []seederv1alpha1.NodeConfig{
					{
						InventoryReference: seederv1alpha1.ObjectReference{
							Name:      "cluster-workflow-test",
							Namespace: "default",
						},
						AddressPoolReference: seederv1alpha1.ObjectReference{
							Name:      "cluster-workflow-test",
							Namespace: "default",
						},
					},
				},
				VIPConfig: seederv1alpha1.VIPConfig{
					AddressPoolReference: seederv1alpha1.ObjectReference{
						Name:      "cluster-workflow-test",
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

	It("ensure cluster status is ClusterTinkHardwareSubmitted", func() {
		Eventually(func() error {
			tmpCluster := &seederv1alpha1.Cluster{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: c.Namespace, Name: c.Name}, tmpCluster); err != nil {
				return err
			}

			if tmpCluster.Status.Status != seederv1alpha1.ClusterTinkHardwareSubmitted {
				return fmt.Errorf("expected status to be tink hardware submitted")
			}

			return nil
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	It("ensure workflow object exists", func() {
		Eventually(func() error {
			workflowObj := &tinkv1alpha1.Workflow{}
			return k8sClient.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, workflowObj)
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	It("delete workflow object", func() {
		Eventually(func() error {
			workflowObj := &tinkv1alpha1.Workflow{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, workflowObj); err != nil {
				return err
			}

			return k8sClient.Delete(ctx, workflowObj)
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	It("ensure workflow object is recreated", func() {
		Eventually(func() error {
			workflowObj := &tinkv1alpha1.Workflow{}
			return k8sClient.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, workflowObj)
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
