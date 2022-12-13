/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	ClusterFinalizer             = "finalizer.cluster.harvesterhci.io"
	DefaultLocalClusterName      = "local"
	DefaultLocalClusterNamespace = "harvester-system"
	DefaultLocalClusterAddress   = "10.53.0.1"
)

var DefaultLocalCluster = types.NamespacedName{Name: DefaultLocalClusterName, Namespace: DefaultLocalClusterName}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ClusterSpec defines the desired state of Cluster
type ClusterSpec struct {
	HarvesterVersion string       `json:"version"`
	ImageURL         string       `json:"imageURL,omitempty"`
	Nodes            []NodeConfig `json:"nodes"`
	VIPConfig        `json:"vipConfig"`
	ClusterConfig    `json:"clusterConfig,omitempty"`
}

type VIPConfig struct {
	AddressPoolReference ObjectReference `json:"addressPoolReference"`
	StaticAddress        string          `json:"staticAddress,omitempty"`
}

type ClusterConfig struct {
	ConfigURL   string   `json:"configURL,omitempty"`
	SSHKeys     []string `json:"sshKeys,omitempty"`
	Nameservers []string `json:"nameservers,omitempty"`
}

type NodeConfig struct {
	InventoryReference   ObjectReference `json:"inventoryReference"`
	AddressPoolReference ObjectReference `json:"addressPoolReference"`
	StaticAddress        string          `json:"staticAddress,omitempty"`
}

type ObjectReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// ClusterStatus defines the observed state of Cluster
type ClusterStatus struct {
	ClusterToken   string                `json:"token,omitempty"`
	Status         ClusterWorkflowStatus `json:"status,omitempty"`
	ClusterAddress string                `json:"clusterAddress,omitempty"`
}

type ClusterWorkflowStatus string

const (
	ClusterConfigReady           ClusterWorkflowStatus = "clusterConfigReady"
	ClusterNodesPatched          ClusterWorkflowStatus = "clusterNodesPatched"
	ClusterTinkHardwareSubmitted ClusterWorkflowStatus = "tinkHardwareCreated"
	ClusterRunning               ClusterWorkflowStatus = "clusterRunning"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="ClusterStatus",type="string",JSONPath=`.status.status`
//+kubebuilder:printcolumn:name="ClusterToken",type="string",JSONPath=`.status.token`
//+kubebuilder:printcolumn:name="ClusterAddress",type="string",JSONPath=`.status.clusterAddress`

// Cluster is the Schema for the clusters API
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec,omitempty"`
	Status ClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ClusterList contains a list of Cluster
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Cluster{}, &ClusterList{})
}
