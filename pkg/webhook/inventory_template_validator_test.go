package webhook

import (
	"testing"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_InventoryTemplateUpdates(t *testing.T) {
	assert := require.New(t)
	var testCases = []struct {
		name      string
		oldObj    *seederv1alpha1.InventoryTemplate
		newObj    *seederv1alpha1.InventoryTemplate
		expectErr bool
	}{
		{
			name: "no changes to spec, as only status gets updated",
			oldObj: &seederv1alpha1.InventoryTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "demo",
				},
				Spec: seederv1alpha1.InventoryTemplateSpec{
					Credentials: &corev1.SecretReference{
						Name:      "demo",
						Namespace: "default",
					},
				},
				Status: seederv1alpha1.InventoryTemplateStatus{
					Status: seederv1alpha1.InventoryTemplateProvisioningError,
				},
			},
			newObj: &seederv1alpha1.InventoryTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "demo",
				},
				Spec: seederv1alpha1.InventoryTemplateSpec{
					Credentials: &corev1.SecretReference{
						Name:      "demo",
						Namespace: "default",
					},
				},
				Status: seederv1alpha1.InventoryTemplateStatus{
					Status: seederv1alpha1.InventoryTemplateProvisioned,
				},
			},
			expectErr: false,
		},
		{
			name: "attempting to change the secret",
			oldObj: &seederv1alpha1.InventoryTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "demo",
				},
				Spec: seederv1alpha1.InventoryTemplateSpec{
					Credentials: &corev1.SecretReference{
						Name:      "demo",
						Namespace: "default",
					},
				},
				Status: seederv1alpha1.InventoryTemplateStatus{
					Status: seederv1alpha1.InventoryTemplateProvisioned,
				},
			},
			newObj: &seederv1alpha1.InventoryTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "demo",
				},
				Spec: seederv1alpha1.InventoryTemplateSpec{
					Credentials: &corev1.SecretReference{
						Name:      "new-demo-secret",
						Namespace: "default",
					},
				},
				Status: seederv1alpha1.InventoryTemplateStatus{
					Status: seederv1alpha1.InventoryTemplateProvisioned,
				},
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		itv := InventoryTemplateValidator{}
		err := itv.Update(nil, tc.oldObj, tc.newObj)
		if tc.expectErr {
			assert.Error(err, "expected error during execution of test case", tc.name)
		} else {
			assert.NoError(err, "expected no error during execution of test case", tc.name)
		}
	}

}
