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

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/tink"
	"github.com/harvester/seeder/pkg/util"
	tinkv1alpha1 "github.com/tinkerbell/tink/pkg/apis/core/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	typedCore "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logr.Logger
}

type clusterReconciler func(context.Context, *seederv1alpha1.Cluster) error

//+kubebuilder:rbac:groups=metal.harvesterhci.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=metal.harvesterhci.io,resources=clusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.harvesterhci.io,resources=clusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Cluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Info("Reconcilling inventory objects", req.Name, req.Namespace)
	// TODO(user): your logic here
	cObj := &seederv1alpha1.Cluster{}

	err := r.Get(ctx, req.NamespacedName, cObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.Error(err, "unable to fetch cluster object")
		return ctrl.Result{}, err
	}

	c := cObj.DeepCopy()
	reconcileList := []clusterReconciler{
		r.generateClusterConfig,
		r.patchNodesAndPools,
		r.createTinkerbellHardware,
		r.reconcileNodes,
		r.markClusterReady,
	}
	deletionReconcileList := []clusterReconciler{
		r.cleanupClusterDeps,
	}

	if c.DeletionTimestamp.IsZero() {
		for _, reconciler := range reconcileList {
			if err := reconciler(ctx, c); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		for _, reconciler := range deletionReconcileList {
			if err := reconciler(ctx, c); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// generateClusterConfig will generate the clusterConfig
func (r *ClusterReconciler) generateClusterConfig(ctx context.Context, c *seederv1alpha1.Cluster) error {
	if c.Status.Status == "" {
		vipPool := &seederv1alpha1.AddressPool{}
		err := r.Get(ctx, types.NamespacedName{Namespace: c.Spec.VIPConfig.AddressPoolReference.Namespace,
			Name: c.Spec.VIPConfig.AddressPoolReference.Name}, vipPool)
		if err != nil {
			return err
		}

		if vipPool.Status.Status != seederv1alpha1.PoolReady {
			return fmt.Errorf("waiting for address pool %s to be ready", vipPool.Name)
		}

		if c.Status.ClusterAddress == "" {
			var addressFound bool
			for address, v := range vipPool.Status.AddressAllocation {
				if v.Kind == seederv1alpha1.KindCluster && v.Name == c.Name && v.Namespace == c.Namespace {
					addressFound = true
					c.Status.ClusterAddress = address
				}
			}
			if !addressFound {
				vip, err := util.AllocateAddress(vipPool.Status.DeepCopy(), c.Spec.VIPConfig.StaticAddress)
				if err != nil {
					return err
				}
				c.Status.ClusterAddress = vip
				// update address allocation
				vipPool.Status.AddressAllocation[vip] = seederv1alpha1.ObjectReferenceWithKind{
					Kind: seederv1alpha1.KindCluster,
					ObjectReference: seederv1alpha1.ObjectReference{
						Name:      c.Name,
						Namespace: c.Namespace,
					},
				}
				if err := r.Status().Update(ctx, vipPool); err != nil {
					return fmt.Errorf("error updating address pool with cluster vip: %v", err)
				}
			}
		}

		c.Status.ClusterToken = util.GenerateRand()
		c.Status.Status = seederv1alpha1.ClusterConfigReady
		return r.Status().Update(ctx, c)
	}
	return nil
}

// patchNodes will patch the node information and associate appropriate events to trigger
// tinkerbell workflows to be generated and reboot initiated
func (r *ClusterReconciler) patchNodesAndPools(ctx context.Context, c *seederv1alpha1.Cluster) error {
	if c.Status.Status == seederv1alpha1.ClusterConfigReady && len(c.Spec.Nodes) > 0 {
		for n, nc := range c.Spec.Nodes {
			pool := &seederv1alpha1.AddressPool{}
			err := r.Get(ctx, types.NamespacedName{Namespace: nc.AddressPoolReference.Namespace,
				Name: nc.AddressPoolReference.Name}, pool)
			if err != nil {
				return fmt.Errorf("error during address pool lookup while configuring nodes: %v", err)
			}

			i := &seederv1alpha1.Inventory{}
			err = r.Get(ctx, types.NamespacedName{Namespace: nc.InventoryReference.Namespace,
				Name: nc.InventoryReference.Name}, i)
			if err != nil {
				return err
			}

			// check that inventory is ready before using it
			if i.Status.Status != seederv1alpha1.InventoryReady {
				return fmt.Errorf("waiting for inventory %s in namespace %s to be ready", i.Name, i.Namespace)
			}

			if util.ConditionExists(i.Status.Conditions, seederv1alpha1.InventoryAllocatedToCluster) {
				continue
			}

			var found bool
			var nodeAddress string
			for address, nodeDetails := range pool.Status.AddressAllocation {
				if nodeDetails.Name == i.Name && nodeDetails.Namespace == i.Namespace {
					found = true
					nodeAddress = address
				}
			}

			if !found {
				if pool.Status.Status != seederv1alpha1.PoolReady {
					return fmt.Errorf("waiting for address pool %s to be ready", pool.Name)
				}
				nodeAddress, err = util.AllocateAddress(pool.Status.DeepCopy(), nc.StaticAddress)
				if err != nil {
					return err
				}

				pool.Status.AddressAllocation[nodeAddress] = seederv1alpha1.ObjectReferenceWithKind{
					ObjectReference: seederv1alpha1.ObjectReference{
						Namespace: i.Namespace,
						Name:      i.Name,
					},
					Kind: seederv1alpha1.KindInventory,
				}
				err = r.Status().Update(ctx, pool)
				if err != nil {
					return fmt.Errorf("error updating address pool after allocation: %v", err)
				}
			}

			i.Status.PXEBootInterface.Address = nodeAddress
			i.Status.PXEBootInterface.Gateway = pool.Spec.Gateway
			i.Status.PXEBootInterface.Netmask = pool.Status.Netmask

			// node password and conditions
			i.Status.GeneratedPassword = util.GenerateRand()
			i.Status.Cluster.Namespace = c.Namespace
			i.Status.Cluster.Name = c.Name
			i.Status.Conditions = util.CreateOrUpdateCondition(i.Status.Conditions, seederv1alpha1.InventoryAllocatedToCluster,
				fmt.Sprintf("node assigned to cluster %s", c.Name))
			i.Status.Conditions = util.RemoveCondition(i.Status.Conditions, seederv1alpha1.InventoryFreed)

			if n == 0 {
				i.Status.Conditions = util.CreateOrUpdateCondition(i.Status.Conditions, seederv1alpha1.HarvesterCreateNode, "Create Mode")
			} else {
				i.Status.Conditions = util.CreateOrUpdateCondition(i.Status.Conditions, seederv1alpha1.HarvesterJoinNode, "Join Mode")
			}

			err = r.Status().Update(ctx, i)
			if err != nil {
				return err
			}

		}

		c.Status.Status = seederv1alpha1.ClusterNodesPatched
		err := r.Status().Update(ctx, c)
		if err != nil {
			return err
		}

		if !controllerutil.ContainsFinalizer(c, seederv1alpha1.ClusterFinalizer) {
			controllerutil.AddFinalizer(c, seederv1alpha1.ClusterFinalizer)
			return r.Update(ctx, c)
		}

	}
	return nil
}

// createTinkerbellHardware will create hardware objects for all nodes in the cluster
func (r *ClusterReconciler) createTinkerbellHardware(ctx context.Context, c *seederv1alpha1.Cluster) error {
	if c.Status.Status == seederv1alpha1.ClusterNodesPatched || c.Status.Status == seederv1alpha1.ClusterTinkHardwareSubmitted || c.Status.Status == seederv1alpha1.ClusterRunning {
		for _, i := range c.Spec.Nodes {
			var hardwareUpdated bool
			inventory := &seederv1alpha1.Inventory{}
			err := r.Get(ctx, types.NamespacedName{Namespace: i.InventoryReference.Namespace, Name: i.InventoryReference.Name}, inventory)
			if err != nil {
				return err
			}

			// if node is missing inventory allocation to cluster
			// then skip the HW generation, as this node doesnt yet have any addresses
			// allocated
			if !util.ConditionExists(inventory.Status.Conditions, seederv1alpha1.InventoryAllocatedToCluster) {
				r.Info("skipping node from hardware generation as it has not yet been processed for allocation to cluster", inventory.Name, inventory.Namespace)
				continue
			}

			hw, err := tink.GenerateHWRequest(inventory, c)
			if err != nil {
				return err
			}
			// create hardware object
			err = controllerutil.SetOwnerReference(c, hw, r.Scheme)
			if err != nil {
				return err
			}

			// create / update hardware object if one already exists
			lookupHw := &tinkv1alpha1.Hardware{}
			err = r.Get(ctx, types.NamespacedName{Namespace: hw.Namespace, Name: hw.Name}, lookupHw)

			if err != nil {
				if apierrors.IsNotFound(err) {
					if err := r.Create(ctx, hw); err != nil {
						return err
					}
					hardwareUpdated = true
				} else {
					return err
				}
			}

			if hardwareUpdated {
				inventory.Status.Conditions = util.CreateOrUpdateCondition(inventory.Status.Conditions, seederv1alpha1.TinkWorkflowCreated, "tink workflow created")
				if err := r.Status().Update(ctx, inventory); err != nil {
					return err
				}
			}
		}

	}

	if c.Status.Status == seederv1alpha1.ClusterNodesPatched {
		c.Status.Status = seederv1alpha1.ClusterTinkHardwareSubmitted
		return r.Status().Update(ctx, c)
	}

	return nil
}

// reconcileNodes will perform housekeeping needed when nodes are added or
// removed from the cluster
func (r *ClusterReconciler) reconcileNodes(ctx context.Context, c *seederv1alpha1.Cluster) error {

	if c.Status.Status == seederv1alpha1.ClusterTinkHardwareSubmitted || c.Status.Status == seederv1alpha1.ClusterRunning {
		items, err := util.ListInventoryAllocatedtoCluster(ctx, r.Client, c)
		if err != nil {
			return err
		}

		// reconcile removed nodes first
		var removedNodes []seederv1alpha1.Inventory
		for _, i := range items {
			var found bool
			var v seederv1alpha1.NodeConfig
			for _, v = range c.Spec.Nodes {
				if i.Namespace == v.InventoryReference.Namespace && i.Name == v.InventoryReference.Name {
					found = true
				}
			}
			if !found {
				removedNodes = append(removedNodes, i)
			}
		}

		for _, i := range removedNodes {
			iObj := &seederv1alpha1.Inventory{}
			err := r.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, iObj)
			if err != nil {
				if apierrors.IsNotFound(err) {
					// inventory removed. Nothing to do
					continue
				} else {
					return err
				}
			}

			// free up address
			a, err := util.FindIPInAddressPools(ctx, r.Client, i.Name, i.Namespace, i.Status.PXEBootInterface.Address)
			if err != nil {
				return err
			}

			if a != nil {
				delete(a.Status.AddressAllocation, i.Status.PXEBootInterface.Address)
				if err := r.Status().Update(ctx, a); err != nil {
					return err
				}
			}
			// need to clean up inventory
			iObj.Status.PXEBootInterface = seederv1alpha1.PXEBootInterface{}
			iObj.Status.Cluster = seederv1alpha1.ObjectReference{}
			iObj.Status.GeneratedPassword = ""
			iObj.Status.Conditions = util.RemoveCondition(iObj.Status.Conditions, seederv1alpha1.InventoryAllocatedToCluster)
			iObj.Status.Conditions = util.RemoveCondition(iObj.Status.Conditions, seederv1alpha1.TinkWorkflowCreated)
			iObj.Status.Conditions = util.RemoveCondition(iObj.Status.Conditions, seederv1alpha1.HarvesterJoinNode)
			iObj.Status.Conditions = util.CreateOrUpdateCondition(iObj.Status.Conditions, seederv1alpha1.InventoryFreed, "")
			if err := r.Status().Update(ctx, iObj); err != nil {
				return err
			}

			// find and clean up hardware object
			hw := &tinkv1alpha1.Hardware{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: iObj.Namespace, Name: iObj.Name}, hw); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				} else {
					return err
				}
			}
			if err := r.Delete(ctx, hw); err != nil {
				return err
			}
		}

		// add nodes to cluster if needed
		var nodesAdded bool
		for _, i := range c.Spec.Nodes {
			iObj := &seederv1alpha1.Inventory{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: i.InventoryReference.Namespace,
				Name: i.InventoryReference.Name}, iObj); err != nil {
				return err
			}
			if !util.ConditionExists(iObj.Status.Conditions, seederv1alpha1.InventoryAllocatedToCluster) || iObj.Status.Cluster.Namespace != c.Namespace || iObj.Status.Cluster.Name != c.Name {
				nodesAdded = true
			}
		}

		if nodesAdded {
			// update status to allow reconcile to happen again from patch nodes and pools phase
			c.Status.Status = seederv1alpha1.ClusterConfigReady
			return r.Status().Update(ctx, c)
		}
	}

	return nil
}

// cleanupClusterDeps will trigger cleanup of nodes and associated infra
func (r *ClusterReconciler) cleanupClusterDeps(ctx context.Context, c *seederv1alpha1.Cluster) error {
	// clean up nodes
	for _, nc := range c.Spec.Nodes {
		var poolmissing, inventorymissing bool
		pool := &seederv1alpha1.AddressPool{}
		err := r.Get(ctx, types.NamespacedName{Namespace: nc.AddressPoolReference.Namespace,
			Name: nc.AddressPoolReference.Name}, pool)
		if err != nil {
			if apierrors.IsNotFound(err) {
				poolmissing = true
			} else {
				return err
			}
		}

		i := &seederv1alpha1.Inventory{}
		err = r.Get(ctx, types.NamespacedName{Namespace: nc.InventoryReference.Namespace,
			Name: nc.InventoryReference.Name}, i)
		if err != nil {
			if apierrors.IsNotFound(err) {
				inventorymissing = true
			} else {
				return err
			}
		}

		if !poolmissing {
			delete(pool.Status.AddressAllocation, i.Status.PXEBootInterface.Address)
			if err := r.Status().Update(ctx, pool); err != nil {
				return err
			}
		}

		if !inventorymissing {
			i.Status.PXEBootInterface = seederv1alpha1.PXEBootInterface{}
			i.Status.Cluster = seederv1alpha1.ObjectReference{}
			i.Status.GeneratedPassword = ""
			i.Status.Conditions = util.RemoveCondition(i.Status.Conditions, seederv1alpha1.InventoryAllocatedToCluster)
			i.Status.Conditions = util.RemoveCondition(i.Status.Conditions, seederv1alpha1.HarvesterJoinNode)
			i.Status.Conditions = util.RemoveCondition(i.Status.Conditions, seederv1alpha1.HarvesterCreateNode)
			i.Status.Conditions = util.CreateOrUpdateCondition(i.Status.Conditions, seederv1alpha1.InventoryFreed, "")
			err = r.Status().Update(ctx, i)
			if err != nil {
				return err
			}
		}

	}

	//cleanup VIP address pool
	if c.Status.ClusterAddress != "" {
		var poolNotFound bool
		pool := &seederv1alpha1.AddressPool{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: c.Spec.VIPConfig.AddressPoolReference.Namespace,
			Name: c.Spec.VIPConfig.AddressPoolReference.Name}, pool); err != nil {
			if apierrors.IsNotFound(err) {
				poolNotFound = true
			} else {
				return err
			}
		}

		if !poolNotFound {
			delete(pool.Status.AddressAllocation, c.Status.ClusterAddress)
			if err := r.Status().Update(ctx, pool); err != nil {
				return err
			}
		}
	}

	if controllerutil.ContainsFinalizer(c, seederv1alpha1.ClusterFinalizer) {
		controllerutil.RemoveFinalizer(c, seederv1alpha1.ClusterFinalizer)
		return r.Update(ctx, c)
	}

	return nil
}

// markClusterReady will use the cluster endpoint and token to try and generate a kubeconfig for target cluster
// and will mark cluster running when the kubeconfig can be generated
func (r *ClusterReconciler) markClusterReady(ctx context.Context, c *seederv1alpha1.Cluster) error {
	// no need to reconcile until the hardware has been submitted
	if c.Status.Status != seederv1alpha1.ClusterTinkHardwareSubmitted {
		return nil
	}

	typedClient, err := genCoreTypedClient(ctx, c)
	if err != nil {
		return err
	}

	nl, err := typedClient.Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	if len(nl.Items) < 1 {
		return fmt.Errorf("api server is running but waiting for one of the nodes to be available")
	}

	c.Status.Status = seederv1alpha1.ClusterRunning
	return r.Status().Update(ctx, c)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&seederv1alpha1.Cluster{}).
		Watches(&source.Kind{Type: &tinkv1alpha1.Hardware{}}, handler.EnqueueRequestsFromMapFunc(func(a client.Object) []reconcile.Request {
			var reconRequest []reconcile.Request
			owners := a.GetOwnerReferences()
			for _, o := range owners {
				if o.Kind == "Cluster" && o.APIVersion == "metal.harvesterhci.io/v1alpha1" {
					reconRequest = append(reconRequest, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: a.GetNamespace(),
							Name:      o.Name,
						},
					})
				}
			}
			return reconRequest
		})).
		Complete(r)
}

func genCoreTypedClient(ctx context.Context, c *seederv1alpha1.Cluster) (*typedCore.CoreV1Client, error) {
	port, ok := c.Labels[seederv1alpha1.OverrideAPIPortLabel]
	if !ok {
		port = seederv1alpha1.DefaultAPIPort
	}

	kcBytes, err := util.GenerateKubeConfig(c.Status.ClusterAddress, port, seederv1alpha1.DefaultAPIPrefix, c.Status.ClusterToken)
	if err != nil {
		return nil, err
	}

	hcClientConfig, err := clientcmd.NewClientConfigFromBytes(kcBytes)
	if err != nil {
		return nil, err
	}

	restConfig, err := hcClientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	return typedCore.NewForConfig(restConfig)
}
