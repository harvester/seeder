package util

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

func Test_GenerateVMPool(t *testing.T) {
	assert := require.New(t)
	diskQuantity, err := resource.ParseQuantity("300Gi")
	assert.NoError(err, "expected no error while parsing diskQuantity")
	memQuantity, err := resource.ParseQuantity("24Gi")
	assert.NoError(err, "expected no error while parsing memoryQuantity")
	inventoryTempate := &seederv1alpha1.InventoryTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dev",
			Namespace: "default",
		},
		Spec: seederv1alpha1.InventoryTemplateSpec{
			VMSpec: seederv1alpha1.VMSpec{
				Namespace: "demo",
				CPU:       1,
				Memory:    memQuantity,
				Count:     3,
				Disks: []seederv1alpha1.DiskConfig{
					{
						Bus:          kubevirtv1.DiskBusVirtio,
						Size:         diskQuantity,
						StorageClass: "harvester-longhorn",
					},
					{
						Bus:          kubevirtv1.DiskBusVirtio,
						Size:         diskQuantity,
						StorageClass: "harvester-longhorn",
					},
				},
				Networks: []seederv1alpha1.NetworkConfig{
					{
						VMNetwork: "default/vlan2017",
						NICModel:  "virtio",
					},
					{
						VMNetwork: "default/vlan2017",
						NICModel:  "virtio",
					},
				},
			},
		},
	}

	vmObjs, err := GenerateVMPool(inventoryTempate)
	assert.NoError(err)
	assert.Len(vmObjs, 3, "exepcted to find 3 VM's defined")
	assert.Equal(vmObjs[0].Namespace, inventoryTempate.Spec.VMSpec.Namespace, "expected vmObj to have namespace matching VMSpec definition")
	assert.Len(vmObjs[0].Spec.DataVolumeTemplates, len(inventoryTempate.Spec.VMSpec.Disks), "expected to find datavolumetemplates matching vm definition")
	assert.Len(vmObjs[0].Spec.Template.Spec.Volumes, len(inventoryTempate.Spec.VMSpec.Disks), "expected to find datavolumetemplates matching vm definition")
	assert.Len(vmObjs[0].Spec.Template.Spec.Networks, len(inventoryTempate.Spec.VMSpec.Networks), "expected to find networks matching vm definition")
	contents, err := yaml.Marshal(vmObjs[0])
	assert.NoError(err)
	fmt.Println(string(contents))

}

func Test_GenerateClusterFromNestedCluster(t *testing.T) {
	assert := require.New(t)
	memory, err := resource.ParseQuantity("8Gi")
	assert.NoError(err, "expected no error while parsing memory quantity")
	disk, err := resource.ParseQuantity("300Gi")
	assert.NoError(err, "expected no error while parsing disk quantity")
	localHarvesterSecretName := "harvester-local"
	storageClass := "harvester-longhorn"
	nadName := "default-harvester-nad"

	n := &seederv1alpha1.NestedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nested-cluster-test",
			Namespace: "default",
		},
		Spec: seederv1alpha1.NestedClusterSpec{
			HarvesterVersion: "v1.3.0",
			ImageURL:         "localhost:5000/v1.3.0",
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
						},
					},
					AddressPoolReference: seederv1alpha1.ObjectReference{
						Name:      "default-address-pool",
						Namespace: "default",
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
						},
					},
					AddressPoolReference: seederv1alpha1.ObjectReference{
						Name:      "default-address-pool",
						Namespace: "default",
					},
				},
			},
		},
	}

	cluster := GenerateClusterFromNestedCluster(n)
	assert.Len(cluster.Spec.Nodes, 8, "expected to find 8 nodes defined in cluster spec")
}
