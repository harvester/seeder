package tink

import (
	"testing"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/rancher/wrangler/pkg/yaml"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_GenerateTemplate(t *testing.T) {
	assert := require.New(t)
	i := &seederv1alpha1.Inventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-inventory",
			Namespace: "harvester-system",
		},
		Spec: seederv1alpha1.InventorySpec{
			PrimaryDisk: "/dev/sda",
		},
	}

	c := &seederv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "harvester-system",
		},
		Spec: seederv1alpha1.ClusterSpec{
			HarvesterVersion: "v1.2.0",
			ImageURL:         "http://imagestore/",
		},
	}

	tinkStackSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tink-stack",
			Namespace: "harvester-system",
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{
						IP: "192.168.1.100",
					},
				},
			},
		},
	}

	template, err := GenerateTemplate(tinkStackSvc, nil, i, c)
	assert.NoError(err, "exppected no error during template generation")
	assert.Equal(template.Name, i.Name, "expected template name to match inventory name")
	assert.Equal(template.Namespace, i.Namespace, "expected template namespace to match inventory namespace")
	assert.NotNil(template.Spec.Data, "expected .spec.data to not be nil")
	workflowObj := &Workflow{}
	err = yaml.Unmarshal([]byte(*template.Spec.Data), workflowObj)
	assert.NoError(err, "expected no error while unmarshalling template.spec.data")
	assert.Len(workflowObj.Tasks, 1, "expected to find 1 task")
	assert.Len(workflowObj.Tasks[0].Actions, 3, "expected to find 3 actions to be performed by the template")

}
