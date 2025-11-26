package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

const (
	InventoryTemplateProvisioned       InventoryTemplateProvisioningStatus = "templateProvisioned"
	InventoryTemplateProvisioningError InventoryTemplateProvisioningStatus = "error"
	SecretKubeconfigFieldKey                                               = "kubeconfig"
	InventoryTemplateFinalizer                                             = "metal.harvesterhci.io/nestedcluster"
	InventoryUUIDLabelKey                                                  = "inventorytemplate.harvesterhci.io/uuid"
	KubeBMCNS                                                              = "kubevirtbmc-system"
	KubeBMCIngressAnnotationKey                                            = "cert-manager.io/issuer"
	KubeBMCIngressAnnotationValue                                          = "kubevirtbmc-selfsigned-issuer"
	KubevirtBMCSecretName                                                  = "kubevirtbmc-secret"
	IngressExposeService                                                   = "ingress-expose"
	KubeSystemNS                                                           = "kube-system"
)

type InventoryTemplateSpec struct {
	Credentials *corev1.SecretReference `json:"credentials"`
	VMSpec      VMSpec                  `json:"vmSpec"`
}

type VMSpec struct {
	// +kubebuilder:default=8
	CPU uint32 `json:"cpu"`
	// +kubebuilder:default="32Gi"
	Memory   resource.Quantity `json:"memory"`
	Disks    []DiskConfig      `json:"disks"`
	Networks []NetworkConfig   `json:"networks"`
	// +kubebuilder:default="default"
	Namespace string `json:"namespace"`
	// +kubebuilder:default=1
	Count int32 `json:"count"`
	// +kubebuilder:default=nginx
	IngressClassName string `json:"ingressClassName"`
}

type DiskConfig struct {
	// +kubebuilder:validation:Enum=virtio;sata;scsi
	// +kubebuilder:default=virtio
	Bus          kubevirtv1.DiskBus `json:"driver"`
	Size         resource.Quantity  `json:"size"`
	StorageClass string             `json:"storageClass"`
}

// NetworkConfig allows users to define VMNetworks to be used with nic interfaces in
// the underlying kubevirt vm
type NetworkConfig struct {
	VMNetwork string `json:"vmNetwork"`
	// +kubebuilder:validation:Enum=e1000;e1000e;igb;ne2k_pci;pcnet;rtl8139;virtio
	// +kubebuilder:default=virtio
	NICModel string `json:"nicModel"`
}

type InventoryTemplateProvisioningStatus string

type InventoryTemplateStatus struct {
	Status  InventoryTemplateProvisioningStatus `json:"status,omitempty"`
	Message string                              `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="InventoryTemplateStatus",type="string",JSONPath=`.status.status`
//+kubebuilder:printcolumn:name="Reason",type="string",JSONPath=`.status.message`

// InventoryTemplate is the Schema for the InventoryTemplate API
type InventoryTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InventoryTemplateSpec   `json:"spec,omitempty"`
	Status InventoryTemplateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// InventoryTemplateList contains a list of InventoryTemplate
type InventoryTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InventoryTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InventoryTemplate{}, &InventoryTemplateList{})
}
