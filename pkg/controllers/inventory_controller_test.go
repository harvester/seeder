package controllers

import (
	"fmt"
	bmaasv1alpha1 "github.com/harvester/bmaas/pkg/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Inventory controller tests", func() {
	i := &bmaasv1alpha1.Inventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sample",
			Namespace: "default",
		},
		Spec: bmaasv1alpha1.InventorySpec{
			PrimaryDisk: "/dev/sda",
			PXEBootInterface: bmaasv1alpha1.PXEBootInterface{
				MacAddress: "xx:xx:xx:xx:xx",
			},
			BaseboardManagementSpec: rufio.BaseboardManagementSpec{
				Connection: rufio.Connection{
					Host:        "localhost",
					Port:        623,
					InsecureTLS: true,
					AuthSecretRef: v1.SecretReference{
						Name:      "sample",
						Namespace: "default",
					},
				},
				Power: rufio.On,
			},
		},
	}

	creds := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sample",
			Namespace: "default",
		},
		StringData: map[string]string{
			"username": "admin",
			"password": "password",
		},
	}

	BeforeEach(func() {
		Eventually(func() error {
			err := k8sClient.Create(ctx, creds)
			if err != nil {
				return err
			}
			err = k8sClient.Create(ctx, i)
			return err
		}, "60s", "5s").ShouldNot(HaveOccurred())
	})

	Context("verify inventory", func() {
		It("run test", func() {
			Eventually(func() error {
				iObj := &bmaasv1alpha1.Inventory{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, iObj)
				if err != nil {
					return err
				}

				if iObj.Status.Status != bmaasv1alpha1.BMCObjectCreated {
					return fmt.Errorf("waiting for baseboard object to be created")
				}
				return nil
			}, "120s", "5s").ShouldNot(HaveOccurred())
		})

	})

	AfterEach(func() {
		Eventually(func() error {
			err := k8sClient.Delete(ctx, creds)
			if err != nil {
				return err
			}
			return k8sClient.Delete(ctx, i)
		}).ShouldNot(HaveOccurred())
	})
})
