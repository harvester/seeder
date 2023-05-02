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

var _ = Describe("Inventory controller and baseboard tests", func() {
	var i *seederv1alpha1.Inventory
	var creds *corev1.Secret

	BeforeEach(func() {
		i = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sample",
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
			err := k8sClient.Create(ctx, creds)
			if err != nil {
				return err
			}
			err = k8sClient.Create(ctx, i)
			return err
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	It("check inventory reconcile", func() {
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

	It("delete baseboardobject", func() {
		Eventually(func() error {
			b := &rufio.Machine{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, b)
			if err != nil {
				return fmt.Errorf("error looking up baseboard object: %v", err)
			}
			err = k8sClient.Delete(ctx, b)
			return err
		}, "30s", "5s").ShouldNot(HaveOccurred())

	})

	It("wait for baseboard to be recreated", func() {
		Eventually(func() error {
			b := &rufio.Machine{}
			return k8sClient.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, b)
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		Eventually(func() error {
			return k8sClient.Delete(ctx, creds)

		}).ShouldNot(HaveOccurred())

		Eventually(func() error {
			return k8sClient.Delete(ctx, i)

		}).ShouldNot(HaveOccurred())

		Eventually(func() error {
			// wait until finalizers have cleaned up objects
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, i)
			if err != nil {
				// object is missing
				if apierrors.IsNotFound(err) {
					return nil
				}
			}
			return fmt.Errorf("waiting for inventory object to be not found")
		}).ShouldNot(HaveOccurred())
	})
})

var _ = Describe("inventory object deletion tests", func() {
	var i *seederv1alpha1.Inventory
	var creds *corev1.Secret

	BeforeEach(func() {
		i = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sample-deletion",
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
							Name:      "sample-deletion",
							Namespace: "default",
						},
					},
				},
			},
		}

		creds = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sample-deletion",
				Namespace: "default",
			},
			StringData: map[string]string{
				"username": "admin",
				"password": "password",
			},
		}

		Eventually(func() error {
			err := k8sClient.Create(ctx, creds)
			if err != nil {
				return err
			}
			err = k8sClient.Create(ctx, i)
			return err
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	It("delete inventory object and check baseboard cleanup", func() {
		Eventually(func() error {
			err := k8sClient.Delete(ctx, i)
			return err
		}, "30s", "5s").ShouldNot(HaveOccurred())

		Eventually(func() error {
			b := &rufio.Machine{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, b)
			if err != nil {
				if apierrors.IsNotFound(err) {
					return nil
				}
			}
			return err
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		Eventually(func() error {
			err := k8sClient.Delete(ctx, creds)
			return err
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})
})

/*var _ = Describe("inventory power action tests", func() {
	var i *seederv1alpha1.Inventory
	var creds *corev1.Secret

	BeforeEach(func() {
		i = &seederv1alpha1.Inventory{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sample-power-action",
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
							Name:      "sample-power-action",
							Namespace: "default",
						},
					},
				},
			},
		}

		creds = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sample-power-action",
				Namespace: "default",
			},
			StringData: map[string]string{
				"username": "admin",
				"password": "password",
			},
		}

		Eventually(func() error {
			err := k8sClient.Create(ctx, creds)
			if err != nil {
				return err
			}
			err = k8sClient.Create(ctx, i)
			return err
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})

	It("submit power action request", func() {
		By("creating a power action", func() {
			Eventually(func() error {
				iObj := &seederv1alpha1.Inventory{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: i.Name, Namespace: i.Namespace}, iObj)
				if err != nil {
					return err
				}

				iObj.Status.PowerAction.ActionRequested = seederv1alpha1.NodePowerActionPowerOn
				return k8sClient.Status().Update(ctx, iObj)
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})

		By("checking inventory obj status got updated", func() {
			Eventually(func() error {
				iObj := &seederv1alpha1.Inventory{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: i.Name, Namespace: i.Namespace}, iObj)
				if err != nil {
					return err
				}
				fmt.Println(iObj.Status.PowerAction)

				jobList := &rufio.JobList{}
				if err := k8sClient.List(ctx, jobList, &client.ListOptions{}); err != nil {
					return fmt.Errorf("error listing jobs: %v", err)
				}

				fmt.Println(jobList.Items)

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

				return nil
			}, "30s", "5s").ShouldNot(HaveOccurred())
		})
	})

	AfterEach(func() {
		Eventually(func() error {
			err := k8sClient.Delete(ctx, creds)
			return err
		}, "30s", "5s").ShouldNot(HaveOccurred())
	})
}) */
