package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PoolStatus string

const (
	PoolReady     PoolStatus = "poolReady"
	PoolExhausted PoolStatus = "poolExhausted"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

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
	Gateway string `json:"gateway"`
}

type AddressStatus struct {
	Status             PoolStatus        `json:"status"`
	StartAddress       string            `json:"startAddress"`
	LastAddress        string            `json:"lastAddress"`
	AvailableAddresses int               `json:"availableAddresses"`
	AddressAllocation  map[string]string `json:"addressAllocation"`
	Netmask            string            `json:"netmask"`
}
