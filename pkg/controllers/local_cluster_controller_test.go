package controllers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"

	rufio "github.com/tinkerbell/rufio/api/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Create and run local cluster tests", func() {
	var i1, i2, i3 *seederv1alpha1.Inventory
	var expectedArray []*seederv1alpha1.Inventory

	BeforeEach(func() {
		i1 = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "local-one",
				Namespace: seederv1alpha1.DefaultLocalClusterNamespace,
				Annotations: map[string]string{
					seederv1alpha1.LocalInventoryAnnotation: "true",
				},
			},
			Spec: seederv1alpha1.InventorySpec{},
		}

		i2 = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "local-two",
				Namespace: seederv1alpha1.DefaultLocalClusterNamespace,
				Annotations: map[string]string{
					seederv1alpha1.LocalInventoryAnnotation: "true",
				},
			},
			Spec: seederv1alpha1.InventorySpec{},
		}

		i3 = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "local-three",
				Namespace: seederv1alpha1.DefaultLocalClusterNamespace,
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
			return k8sClient.Create(ctx, i1)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, i2)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, i3)
		}, "30s", "5s").ShouldNot(HaveOccurred())
		expectedArray = append(expectedArray, i1, i2)
	})

	It("run local cluster tests", func() {
		By("checking local cluster status", func() {
			Eventually(func() error {
				localObj := &seederv1alpha1.Cluster{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: seederv1alpha1.DefaultLocalClusterNamespace, Name: seederv1alpha1.DefaultLocalClusterName}, localObj)
				if err != nil {
					return err
				}

				if localObj.Status.Status != seederv1alpha1.ClusterRunning {
					return fmt.Errorf("expected to find cluster status %s but got %s", seederv1alpha1.ClusterRunning, localObj.Status.Status)
				}

				if localObj.Status.ClusterAddress != seederv1alpha1.DefaultLocalClusterAddress {
					return fmt.Errorf("expected to find cluster address %s but got %s", seederv1alpha1.DefaultLocalClusterAddress, localObj.Status.ClusterAddress)
				}

				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("checking nodes added to local cluster", func() {
			Eventually(func() error {
				localCluster := &seederv1alpha1.Cluster{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: seederv1alpha1.DefaultLocalClusterNamespace, Name: seederv1alpha1.DefaultLocalClusterName}, localCluster)
				if err != nil {
					return err
				}
				for _, i := range expectedArray {
					var found bool
					for _, v := range localCluster.Spec.Nodes {
						if v.InventoryReference.Name == i.Name && v.InventoryReference.Namespace == i.Namespace {
							found = true
						}
					}
					if !found {
						return fmt.Errorf("did not find inventory %s in localCluster", i.Name)
					}
				}
				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("checking finalizer on local cluster inventory", func() {
			Eventually(func() error {
				inventoryObj := &seederv1alpha1.Inventory{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i1.Namespace, Name: i1.Name}, inventoryObj)
				if err != nil {
					return err
				}

				if !controllerutil.ContainsFinalizer(inventoryObj, seederv1alpha1.InventoryFinalizer) {
					return fmt.Errorf("expected to find finalizer on inventory obj one")
				}

				return nil
			}, "60s", "5s").ShouldNot(HaveOccurred())
		})

		By("checking machine object exists", func() {
			Eventually(func() error {
				machineObj := &rufio.Machine{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i1.Namespace, Name: i1.Name}, machineObj)
				return err
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("delete machine", func() {
			Eventually(func() error {
				machineObj := &rufio.Machine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      i1.Name,
						Namespace: i1.Namespace,
					},
				}
				err := k8sClient.Delete(ctx, machineObj)
				return err
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("checking machine object is recreated", func() {
			Eventually(func() error {
				machineObj := &rufio.Machine{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i1.Namespace, Name: i1.Name}, machineObj)
				return err
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("deleting one inventory object", func() {
			Eventually(func() error {
				return k8sClient.Delete(ctx, i1)
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("checking inventory has been removed from local cluster", func() {
			Eventually(func() error {
				localCluster := &seederv1alpha1.Cluster{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: seederv1alpha1.DefaultLocalClusterNamespace, Name: seederv1alpha1.DefaultLocalClusterName}, localCluster)
				if err != nil {
					return err
				}

				if len(localCluster.Spec.Nodes) != 1 {
					return fmt.Errorf("expected to find only one node in local-cluster")
				}

				if localCluster.Spec.Nodes[0].InventoryReference.Name != i2.Name {
					return fmt.Errorf("expected to find only inventory object i2")
				}
				return nil
			}, "30s", "5s")
		})

		By("checking machine has been removed", func() {
			Eventually(func() error {
				machineObj := &rufio.Machine{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i1.Namespace, Name: i1.Name}, machineObj)
				if err != nil {
					if apierrors.IsNotFound(err) {
						return nil
					}
				}
				return err
			})
		})

		By("update status of inventory", func() {
			Eventually(func() error {
				inventoryObj := &seederv1alpha1.Inventory{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i2.Namespace, Name: i2.Name}, inventoryObj)
				if err != nil {
					return err
				}

				statusString := `{"ownerCluster": {"name": "local","namespace": "harvester-system"},"pxeBootConfig": {"address": "172.19.108.4", "gateway":"", "netmask":""},"status": "inventoryNodeReady", "generatedPassword":"", "hardwareID":""}`
				b64statusString := base64.StdEncoding.EncodeToString([]byte(statusString))
				inventoryObj.Annotations[seederv1alpha1.LocalInventoryStatusAnnotation] = b64statusString
				return k8sClient.Update(ctx, inventoryObj)
			})
		})

		By("reconcile status of inventory", func() {
			Eventually(func() error {
				inventoryObj := &seederv1alpha1.Inventory{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i2.Namespace, Name: i2.Name}, inventoryObj)
				if err != nil {
					return err
				}

				b64statusString, ok := inventoryObj.Annotations[seederv1alpha1.LocalInventoryStatusAnnotation]
				if !ok {
					return fmt.Errorf("expected to find inventorystatusannotation")
				}

				statusString, err := base64.StdEncoding.DecodeString(b64statusString)
				if err != nil {
					return err
				}
				annotationStatus := &seederv1alpha1.InventoryStatus{}
				err = json.Unmarshal(statusString, annotationStatus)
				if err != nil {
					return fmt.Errorf("error unmarshalling inventory status from string: %v", err)
				}

				if !reflect.DeepEqual(annotationStatus, inventoryObj.Status) {
					return fmt.Errorf("inventory status doesnt not match embedded json")
				}
				return nil
			})
		})

	})

	AfterEach(func() {
		Eventually(func() error {
			localCluster := &seederv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      seederv1alpha1.DefaultLocalClusterName,
					Namespace: seederv1alpha1.DefaultLocalClusterNamespace,
				},
			}
			return k8sClient.Delete(ctx, localCluster)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, i1)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, i2)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, i3)
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})
})

func createHarvesterNamespace(ctx context.Context, k8sClient client.Client) error {
	ns := &corev1.Namespace{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: "harvester-system", Namespace: ""}, ns)
	if err != nil {
		if apierrors.IsNotFound(err) {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "harvester-system",
				},
			}
			return k8sClient.Create(ctx, ns)
		}
	}
	return err
}
