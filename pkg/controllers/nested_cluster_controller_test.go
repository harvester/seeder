package controllers

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

var _ = Describe("NestedCluster Controller", func() {
	var n *seederv1alpha1.NestedCluster
	var a *seederv1alpha1.AddressPool

	memory, err := resource.ParseQuantity("8Gi")
	Expect(err).NotTo(HaveOccurred())
	disk, err := resource.ParseQuantity("300Gi")
	Expect(err).NotTo(HaveOccurred())

	BeforeEach(func() {

		a = &seederv1alpha1.AddressPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nested-cluster-address-pool",
				Namespace: "default",
			},
			Spec: seederv1alpha1.AddressSpec{
				CIDR:    "192.168.1.1/29",
				Gateway: "192.168.1.7",
			},
		}

		n = &seederv1alpha1.NestedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nested-cluster-test",
				Namespace: "default",
			},
			Spec: seederv1alpha1.NestedClusterSpec{
				HarvesterVersion: "v1.3.0",
				ImageURL:         "localhost:5000/v1.3.0",
				VIPConfig: seederv1alpha1.VIPConfig{
					AddressPoolReference: seederv1alpha1.ObjectReference{
						Name:      a.Name,
						Namespace: a.Namespace,
					},
				},
				InventoryTemplateConfig: []seederv1alpha1.InventoryTemplateConfig{
					{
						Name: "template-1",
						InventoryTemplateSpec: seederv1alpha1.InventoryTemplateSpec{
							Credentials: &corev1.SecretReference{
								Name:      localHarvesterSecretName,
								Namespace: "default",
							},
							VMSpec: seederv1alpha1.VMSpec{
								CPU:    4,
								Memory: memory,
								Disks: []seederv1alpha1.DiskConfig{
									{
										Bus:          "virtio",
										Size:         disk,
										StorageClass: storageClass,
									},
								},
								Networks: []seederv1alpha1.NetworkConfig{
									{
										NICModel:  "virtio",
										VMNetwork: nadName,
									},
								},
								IngressClassName: "nginx",
								Count:            4,
								Namespace:        "default",
							},
						},
						AddressPoolReference: seederv1alpha1.ObjectReference{
							Name:      a.Name,
							Namespace: a.Namespace,
						},
					},
					{
						Name: "template-2",
						InventoryTemplateSpec: seederv1alpha1.InventoryTemplateSpec{
							Credentials: &corev1.SecretReference{
								Name:      localHarvesterSecretName,
								Namespace: "default",
							},
							VMSpec: seederv1alpha1.VMSpec{
								CPU:    4,
								Memory: memory,
								Disks: []seederv1alpha1.DiskConfig{
									{
										Bus:          "virtio",
										Size:         disk,
										StorageClass: storageClass,
									},
								},
								Networks: []seederv1alpha1.NetworkConfig{
									{
										NICModel:  "virtio",
										VMNetwork: nadName,
									},
								},
								IngressClassName: "nginx",
								Count:            4,
								Namespace:        "default",
							},
						},
						AddressPoolReference: seederv1alpha1.ObjectReference{
							Name:      a.Name,
							Namespace: a.Namespace,
						},
					},
				},
			},
		}

		Eventually(func() error {
			return k8sClient.Create(ctx, a)
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Create(ctx, n)
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	It("ensure nested cluster and associated inventory templates are created", func() {

		By("checking inventory templates exist", func() {
			Eventually(func() error {
				templates := &seederv1alpha1.InventoryTemplateList{}
				err := k8sClient.List(ctx, templates, client.InNamespace(n.Namespace), client.MatchingLabels{seederv1alpha1.NestedClusterUIDLabelKey: string(n.GetUID())})
				if err != nil {
					return err
				}

				if len(templates.Items) != len(n.Spec.InventoryTemplateConfig) {
					return fmt.Errorf("expected %d inventory templates, but got %d", len(n.Spec.InventoryTemplateConfig), len(templates.Items))
				}
				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})
	})

})
