package controllers

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/util"
)

var _ = Describe("test local node controller", func() {
	var n *corev1.Node
	var i *seederv1alpha1.Inventory
	var creds *corev1.Secret
	BeforeEach(func() {
		n = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "power-test",
			},
			Spec: corev1.NodeSpec{},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{
					{
						Type:    corev1.NodeInternalIP,
						Address: "127.0.0.1",
					},
				},
			},
		}

		i = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "power-test",
				Namespace: seederv1alpha1.DefaultLocalClusterNamespace,
				Annotations: map[string]string{
					seederv1alpha1.LocalInventoryAnnotation: "true",
					seederv1alpha1.LocalInventoryNodeName:   n.Name,
				},
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
							Name:      "sample",
							Namespace: "default",
						},
					},
				},
			},
		}

		creds = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sample",
				Namespace: "default",
			},
			StringData: map[string]string{
				"username": "admin",
				"password": "password",
			},
		}

		Eventually(func() error {
			return util.SetupLocalCluster(ctx, k8sClient)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, i)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, creds)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			if err := k8sClient.Create(ctx, n); err != nil {
				return err
			}

			nObj := &corev1.Node{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: i.Name}, nObj); err != nil {
				return err
			}
			nObj.Status.Addresses = n.Status.Addresses
			return k8sClient.Status().Update(ctx, nObj)
		}, "30s", "5s").ShouldNot(HaveOccurred())

	})

	AfterEach(func() {
		Eventually(func() error {
			return k8sClient.Delete(ctx, i)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, n)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, creds)
		}, "30s", "5s").ShouldNot(HaveOccurred())

	})

	It("run node power tests", func() {

		By("ensuring machine is created", func() {
			Eventually(func() error {
				machine := &rufio.Machine{}
				return k8sClient.Get(ctx, types.NamespacedName{Name: n.Name, Namespace: seederv1alpha1.DefaultLocalClusterNamespace}, machine)
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("checking inventory is ready", func() {
			Eventually(func() error {
				iObj := &seederv1alpha1.Inventory{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, iObj)
				if err != nil {
					return err
				}

				if iObj.Status.Status != seederv1alpha1.InventoryReady {
					return fmt.Errorf("waiting for baseboard object to be created. Current status %v", iObj)
				}
				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("checking inventory is allocated to cluster", func() {
			Eventually(func() error {
				iObj := &seederv1alpha1.Inventory{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, iObj)
				if err != nil {
					return err
				}

				if !util.ConditionExists(iObj, seederv1alpha1.InventoryAllocatedToCluster) {
					return fmt.Errorf("waiting for inventory to be allocated to cluster")
				}
				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("creating a power action", func() {
			Eventually(func() error {
				iObj := &seederv1alpha1.Inventory{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: i.Name, Namespace: i.Namespace}, iObj)
				if err != nil {
					return err
				}

				iObj.Spec.PowerActionRequested = seederv1alpha1.NodePowerActionPowerOn
				return k8sClient.Update(ctx, iObj)
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("checking inventory obj status got updated", func() {
			Eventually(func() error {
				iObj := &seederv1alpha1.Inventory{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: i.Name, Namespace: i.Namespace}, iObj)
				if err != nil {
					return err
				}
				fmt.Println(iObj.Status)

				if iObj.Status.PowerAction.LastJobName == "" {
					return fmt.Errorf("expected to find last job name to be populated")
				}

				powerActionJob := &rufio.Job{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: iObj.Namespace, Name: iObj.Status.PowerAction.LastJobName}, powerActionJob); err != nil {
					return fmt.Errorf("error querying power action job: %v", err)
				}

				if iObj.Status.PowerAction.LastActionStatus != seederv1alpha1.NodeJobComplete {
					return fmt.Errorf("expected to find last action status to be %v", seederv1alpha1.NodeJobComplete)
				}

				fmt.Println(iObj.Status.Conditions)
				if util.ConditionExists(iObj, seederv1alpha1.BMCJobSubmitted) {
					return fmt.Errorf("expected BMCJobSubmitted condition to be removed")
				}
				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})
	})
})
