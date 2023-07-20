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
	"github.com/rancher/wrangler/pkg/condition"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type InventoryWorkflowStatus string

type TinkWorkflowStatus string

type TaskWorkflowStatus string

type ConditionType condition.Cond

const (
	KindCluster   string = "cluster"
	KindInventory string = "inventory"
)

const (
	InventoryFinalizer       = "finalizer.inventory.metal.harvesterhci.io"
	LocalInventoryAnnotation = "metal.harvesterhci.io/local-inventory"
	LocalInventoryNodeName   = "metal.harvesterhci.io/local-node-name"
)

const (
	InventoryReady InventoryWorkflowStatus = "inventoryNodeReady"
)

const (
	BMCObjectCreated            condition.Cond = "bmcObjectCreated"
	BMCJobSubmitted             condition.Cond = "bmcJobSubmitted"
	BMCJobComplete              condition.Cond = "bmcJobCompleted"
	BMCJobError                 condition.Cond = "bmcJobErrorr"
	TinkWorkflowCreated         condition.Cond = "tinkWorkflowCreated"
	InventoryAllocatedToCluster condition.Cond = "inventoryAllocatedToCluster"
	InventoryFreed              condition.Cond = "inventoryFreed"
	HarvesterCreateNode         condition.Cond = "harvesterCreateNode"
	HarvesterJoinNode           condition.Cond = "harvesterJoinNode"
	MachineNotContactable       condition.Cond = "machineNotContactable"
)

// InventorySpec defines the desired state of Inventory
type InventorySpec struct {
	PrimaryDisk                   string            `json:"primaryDisk"`
	ManagementInterfaceMacAddress string            `json:"managementInterfaceMacAddress"`
	BaseboardManagementSpec       rufio.MachineSpec `json:"baseboardSpec"`
	Events                        `json:"events"`
	PowerActionRequested          string `json:"powerActionRequested,omitempty"`
}

type BMCSecretReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type PXEBootInterface struct {
	Address     string   `json:"address,omitempty"`
	Gateway     string   `json:"gateway,omitempty"`
	Netmask     string   `json:"netmask,omitempty"`
	NameServers []string `json:"nameServers,omitempty"`
}

// InventoryStatus defines the observed state of Inventory
type InventoryStatus struct {
	Status            InventoryWorkflowStatus `json:"status,omitempty"`
	GeneratedPassword string                  `json:"generatedPassword,omitempty"`
	HardwareID        string                  `json:"hardwareID,omitempty"`
	Conditions        []Conditions            `json:"conditions,omitempty"`
	PXEBootInterface  `json:"pxeBootConfig,omitempty"`
	Cluster           ObjectReference    `json:"ownerCluster,omitempty"`
	PowerAction       PowerActionDetails `json:"powerAction,omitempty"`
}

type Conditions struct {
	Type               condition.Cond         `json:"type"`
	Status             corev1.ConditionStatus `json:"status"`
	LastUpdateTime     string                 `json:"lastUpdateTime,omitempty"`
	LastTransitionTime string                 `json:"lastTransitionTime,omitempty"`
	Reason             string                 `json:"reason,omitempty"`
	Message            string                 `json:"message,omitempty"`
}

type Events struct {
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`
	// +kubebuilder:default:="1h"
	// +kubebuilder:validation:Format:=duration
	PollingInterval string `json:"pollingInterval,omitempty"`
}

type PowerActionDetails struct {
	LastActionStatus    string `json:"actionStatus,omitempty"`
	LastActionRequested string `json:"lastActionRequested,omitempty"`
	LastJobName         string `json:"lastJobName,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="InventoryStatus",type="string",JSONPath=`.status.status`
//+kubebuilder:printcolumn:name="GeneratedPassword",type="string",JSONPath=`.status.generatedPassword`
//+kubebuilder:printcolumn:name="AllocatedNodeAddress",type="string",JSONPath=`.status.pxeBootConfig.address`

// Inventory is the Schema for the inventories API
type Inventory struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InventorySpec   `json:"spec,omitempty"`
	Status InventoryStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// InventoryList contains a list of Inventory
type InventoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Inventory `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Inventory{}, &InventoryList{})
}
