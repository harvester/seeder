package controllers

import (
	"fmt"

	"github.com/harvester/seeder/pkg/util"

	rufio "github.com/tinkerbell/rufio/api/v1alpha1"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = FDescribe("test local node controller", func() {
	var n *corev1.Node
	var i *seederv1alpha1.Inventory

	BeforeEach(func() {
		n = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "power-test",
			},
			Spec: corev1.NodeSpec{},
		}

		i = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      n.Name,
				Namespace: seederv1alpha1.DefaultLocalClusterNamespace,
				Annotations: map[string]string{
					seederv1alpha1.LocalInventoryAnnotation:       "true",
					seederv1alpha1.LocalInventoryStatusAnnotation: "eyJvd25lckNsdXN0ZXIiOiB7Im5hbWUiOiAibG9jYWwiLCJuYW1lc3BhY2UiOiAiaGFydmVzdGVyLXN5c3RlbSJ9LCJweGVCb290Q29uZmlnIjogeyJhZGRyZXNzIjogIjE3Mi4xOS4xMDguNCIsICJnYXRld2F5IjoiIiwgIm5ldG1hc2siOiIifSwic3RhdHVzIjogImludmVudG9yeU5vZGVSZWFkeSIsICJnZW5lcmF0ZWRQYXNzd29yZCI6IiIsICJoYXJkd2FyZUlEIjoiIn0K",
				},
			},
			Spec: seederv1alpha1.InventorySpec{},
		}

		Eventually(func() error {
			return createHarvesterNamespace(ctx, k8sClient)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return util.SetupLocalCluster(ctx, k8sClient)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, i)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, n)
		}, "30s", "5s").ShouldNot(HaveOccurred())

	})

	AfterEach(func() {
		Eventually(func() error {
			return k8sClient.Delete(ctx, i)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, n)
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	It("run node power tests", func() {

		By("ensuring machine is created", func() {
			Eventually(func() error {
				machine := &rufio.Machine{}
				return k8sClient.Get(ctx, types.NamespacedName{Name: n.Name, Namespace: seederv1alpha1.DefaultLocalClusterNamespace}, machine)
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("annotate node for shutdown", func() {
			Eventually(func() error {
				nodeObj := &corev1.Node{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: n.Name, Namespace: n.Namespace}, nodeObj); err != nil {
					return err
				}
				if nodeObj.Annotations == nil {
					nodeObj.Annotations = make(map[string]string)
				}
				nodeObj.Annotations[seederv1alpha1.NodeActionRequested] = seederv1alpha1.NodePowerActionShutdown
				return k8sClient.Update(ctx, nodeObj)
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("ensuring correct annotations are updated for shutdown", func() {
			Eventually(func() error {
				nodeObj := &corev1.Node{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: n.Name, Namespace: n.Namespace}, nodeObj); err != nil {
					return err
				}

				_, ok := nodeObj.Annotations[seederv1alpha1.NodeActionRequested]
				if ok {
					return fmt.Errorf("expected NodeActionRequested annotation to be removed")
				}

				_, ok = nodeObj.Annotations[seederv1alpha1.NodePowerActionJobName]
				if !ok {
					return fmt.Errorf("expected NoPowerActionJobName annotation to be removed")
				}

				_, ok = nodeObj.Annotations[seederv1alpha1.NodeActionStatus]
				if !ok {
					return fmt.Errorf("expected to find NodeActionStatus annotation")
				}

				v, ok := nodeObj.Annotations[seederv1alpha1.NodeLastActionRequest]
				if !ok {
					return fmt.Errorf("expected to find NodeLastActionRequest annotation")
				}

				if v != seederv1alpha1.NodePowerActionShutdown {
					return fmt.Errorf("expected to find NodeLastActionRequest to be shutdwon by got %s", v)
				}
				return nil
			}, "60s", "5s").ShouldNot(HaveOccurred())
		})

		By("ensure shutdown job has been removed", func() {
			Eventually(func() error {
				job := &rufio.Job{}
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: seederv1alpha1.DefaultLocalClusterNamespace, Name: "power-test-shutdown"}, job)
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("annotate node for poweron", func() {
			Eventually(func() error {
				nodeObj := &corev1.Node{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: n.Name, Namespace: n.Namespace}, nodeObj); err != nil {
					return err
				}
				if nodeObj.Annotations == nil {
					nodeObj.Annotations = make(map[string]string)
				}
				nodeObj.Annotations[seederv1alpha1.NodeActionRequested] = seederv1alpha1.NodePowerActionPowerOn
				return k8sClient.Update(ctx, nodeObj)
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("ensuring correct annotations are updated for poweron", func() {
			Eventually(func() error {
				nodeObj := &corev1.Node{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: n.Name, Namespace: n.Namespace}, nodeObj); err != nil {
					return err
				}

				GinkgoWriter.Println(nodeObj.Annotations)
				_, ok := nodeObj.Annotations[seederv1alpha1.NodeActionRequested]
				if ok {
					return fmt.Errorf("expected NodeActionRequested annotation to be removed")
				}

				_, ok = nodeObj.Annotations[seederv1alpha1.NodePowerActionJobName]
				if !ok {
					return fmt.Errorf("expected NoPowerActionJobName annotation to be removed")
				}

				_, ok = nodeObj.Annotations[seederv1alpha1.NodeActionStatus]
				if !ok {
					return fmt.Errorf("expected to find NodeActionStatus annotation")
				}

				v, ok := nodeObj.Annotations[seederv1alpha1.NodeLastActionRequest]
				if !ok {
					return fmt.Errorf("expected to find NodeLastActionRequest annotation")
				}

				if v != seederv1alpha1.NodePowerActionPowerOn {
					return fmt.Errorf("expected to find NodeLastActionRequest to be poweron but got %s", v)
				}
				return nil
			}, "60s", "5s").ShouldNot(HaveOccurred())
		})

		By("ensure poweron job has been removed", func() {
			Eventually(func() error {
				job := &rufio.Job{}
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: seederv1alpha1.DefaultLocalClusterNamespace, Name: "power-test-poweron"}, job)
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

	})
})
