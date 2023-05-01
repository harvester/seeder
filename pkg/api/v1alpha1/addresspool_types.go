package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PoolStatus string

const (
	PoolReady     PoolStatus = "poolReady"
	PoolExhausted PoolStatus = "poolExhausted"
)

const (
	AddressPoolFinalizer = "finalizer.addresspool.harvesterhci.io"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="AddressPoolStatus",type="string",JSONPath=`.status.status`
//+kubebuilder:printcolumn:name="StartAddress",type="string",JSONPath=`.status.startAddress`
//+kubebuilder:printcolumn:name="LastAddress",type="string",JSONPath=`.status.lastAddress`
//+kubebuilder:printcolumn:name="NetMask",type="string",JSONPath=`.status.netmask`

// AddressPool is the CRD for maintaining Aaddress pools for Harvester nodes and VIP's
type AddressPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AddressSpec   `json:"spec,omitempty"`
	Status AddressStatus `json:"status,omitempty"`
}

type AddressSpec struct {
	CIDR    string `json:"cidr"`
	Netmask string `json:"netmask,omitempty"`
	Gateway string `json:"gatewayg"`
}

type AddressStatus struct {
	Status             PoolStatus                         `json:"status"`
	StartAddress       string                             `json:"startAddress"`
	LastAddress        string                             `json:"lastAddress"`
	AvailableAddresses int                                `json:"availableAddresses"`
	AddressAllocation  map[string]ObjectReferenceWithKind `json:"addressAllocation"`
	Netmask            string                             `json:"netmask"`
}

type ObjectReferenceWithKind struct {
	ObjectReference `json:",inline"`
	Kind            string `json:"kind"`
}

//+kubebuilder:object:root=true

// ClusterList contains a list of Cluster
type AddressPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AddressPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AddressPool{}, &AddressPoolList{})
}
