package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	NestedClusterFinalizer   = "metal.harvesterhci.io/nestedcluster"
	NestedClusterUIDLabelKey = "nestedcluster.harvesterhci.io/uuid"
)

// NestedClusterSpec defines the state of underlying Cluster object
// generted by the nested cluster controller
type NestedClusterSpec struct {
	HarvesterVersion        string                    `json:"version"`
	ImageURL                string                    `json:"imageURL"`
	InventoryTemplateConfig []InventoryTemplateConfig `json:"inventoryTemplateConfig"`
	VIPConfig               `json:"vipConfig"`
	ClusterConfig           `json:"clusterConfig"`
}

// InventoryTemplateConfig defines the inventory template
// and eventually geenerates the inventory objects and configures a cluster
// using the generated inventory and address pool references
type InventoryTemplateConfig struct {
	Name                  string                `json:"name"`
	InventoryTemplateSpec InventoryTemplateSpec `json:"inventoryTemplateSpec"`
	AddressPoolReference  ObjectReference       `json:"addressPoolReference"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="ClusterStatus",type="string",JSONPath=`.status.status`
//+kubebuilder:printcolumn:name="ClusterToken",type="string",JSONPath=`.status.token`
//+kubebuilder:printcolumn:name="ClusterAddress",type="string",JSONPath=`.status.clusterAddress`

// Cluster is the Schema for the clusters API
type NestedCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NestedClusterSpec `json:"spec,omitempty"`
	Status ClusterStatus     `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ClusterList contains a list of Cluster
type NestedClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NestedCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NestedCluster{}, &NestedClusterList{})
}
