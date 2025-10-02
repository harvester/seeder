/*
Copyright 2024.

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
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/rancher/wrangler/pkg/condition"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	typedCore "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/tink"
	"github.com/harvester/seeder/pkg/util"
)

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logr.Logger
	mutex *sync.Mutex
}

const (
	DefaultDeletionReconcileInterval = 30 * time.Second
)

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

	// ignore the local cluster
	if c.Name == seederv1alpha1.DefaultLocalClusterName && c.Namespace == seederv1alpha1.DefaultLocalClusterNamespace {
		return ctrl.Result{}, nil
	}

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
		if !controllerutil.ContainsFinalizer(c, seederv1alpha1.ClusterFinalizer) {
			controllerutil.AddFinalizer(c, seederv1alpha1.ClusterFinalizer)
			return ctrl.Result{}, r.Update(ctx, c)
		}
		for _, reconciler := range reconcileList {
			if err := reconciler(ctx, c); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		for _, reconciler := range deletionReconcileList {
			if err := reconciler(ctx, c); err != nil {
				return ctrl.Result{RequeueAfter: DefaultDeletionReconcileInterval}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// generateClusterConfig will generate the clusterConfig
func (r *ClusterReconciler) generateClusterConfig(ctx context.Context, cObj *seederv1alpha1.Cluster) error {
	c := cObj.DeepCopy()
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
				if err := r.lockedAddressPoolUpdate(ctx, vipPool); err != nil {
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
func (r *ClusterReconciler) patchNodesAndPools(ctx context.Context, cObj *seederv1alpha1.Cluster) error {
	c := cObj.DeepCopy()
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

			if util.ConditionExists(i, seederv1alpha1.InventoryAllocatedToCluster) {
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
				err = r.lockedAddressPoolUpdate(ctx, pool)
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
			util.CreateOrUpdateCondition(i, seederv1alpha1.InventoryAllocatedToCluster,
				fmt.Sprintf("node assigned to cluster %s", c.Name))
			util.RemoveCondition(i, seederv1alpha1.InventoryFreed)

			if n == 0 {
				util.CreateOrUpdateCondition(i, seederv1alpha1.HarvesterCreateNode, "Create Mode")
			} else {
				util.CreateOrUpdateCondition(i, seederv1alpha1.HarvesterJoinNode, "Join Mode")
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

	}
	return nil
}

// createTinkerbellHardware will create hardware objects for all nodes in the cluster
func (r *ClusterReconciler) createTinkerbellHardware(ctx context.Context, cObj *seederv1alpha1.Cluster) error {
	c := cObj.DeepCopy()
	if c.Status.Status == seederv1alpha1.ClusterNodesPatched || c.Status.Status == seederv1alpha1.ClusterTinkHardwareSubmitted || c.Status.Status == seederv1alpha1.ClusterRunning {

		// check to see if the service for tink-stack is ready
		tinkStackService := &corev1.Service{}
		err := r.Get(ctx, types.NamespacedName{Name: seederv1alpha1.DefaultTinkStackService, Namespace: deploymentNamespace}, tinkStackService)
		if err != nil {
			return fmt.Errorf("error fetching svc %s in ns %s: %v", seederv1alpha1.DefaultTinkStackService, seederv1alpha1.DefaultLocalClusterNamespace, err)
		}

		seederDeploymentService := &corev1.Service{}
		err = r.Get(ctx, types.NamespacedName{Name: seederv1alpha1.DefaultSeederDeploymentService, Namespace: deploymentNamespace}, seederDeploymentService)

		if err != nil {
			return fmt.Errorf("error fetching svc %s in ns %s: %v", seederv1alpha1.DefaultSeederDeploymentService, seederv1alpha1.DefaultLocalClusterNamespace, err)
		}

		for _, i := range c.Spec.Nodes {
			inventory := &seederv1alpha1.Inventory{}
			err := r.Get(ctx, types.NamespacedName{Namespace: i.InventoryReference.Namespace, Name: i.InventoryReference.Name}, inventory)
			if err != nil {
				return err
			}

			// if node is missing inventory allocation to cluster
			// then skip the HW generation, as this node doesnt yet have any addresses
			// allocated
			if !util.ConditionExists(inventory, seederv1alpha1.InventoryAllocatedToCluster) {
				r.Info("skipping node from hardware generation as it has not yet been processed for allocation to cluster", inventory.Name, inventory.Namespace)
				continue
			}

			// tinkStack Service exposes Hegel endpoint
			// seederDeploymentService exposes the api endpoint to update hardware objects
			hw, err := tink.GenerateHWRequest(inventory, c, seederDeploymentService, tinkStackService)
			if err != nil {
				return err
			}
			// create hardware object
			err = controllerutil.SetOwnerReference(c, hw, r.Scheme)
			if err != nil {
				return err
			}

			// create / update hardware object if one already exists
			err = r.createOrUpdateHardware(ctx, hw, inventory)
			if err != nil {
				return err
			}
		}

	}

	return nil
}

// reconcileNodes will perform housekeeping needed when nodes are added or
// removed from the cluster
func (r *ClusterReconciler) reconcileNodes(ctx context.Context, cObj *seederv1alpha1.Cluster) error {
	c := cObj.DeepCopy()
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

			// skip cleanup of node allocated to another cluster
			if iObj.Status.Cluster.Name != c.Name || iObj.Status.Cluster.Namespace != c.Namespace {
				continue
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
			ok, err := r.ensureInventoryIsShutdown(ctx, c, iObj)
			if err != nil {
				return fmt.Errorf("error ensuring inventory %s is shutdown %v", i.Name, err)
			}

			if !ok {
				return fmt.Errorf("waiting for inventory %s to be shutdown", i.Name)
			}
			// fetch and clear last job request
			err = r.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, iObj)
			if err != nil {
				return fmt.Errorf("error getting inventory during cleanupClusterDeps %s %v", i.Name, err)
			}
			iObj.Spec.PowerActionRequested = ""
			err = r.Update(ctx, iObj)
			if err != nil {
				return fmt.Errorf("error resetting powerActionRequested on inventory %s during cleanupClusterDeps %v", i.Name, err)
			}
			// need to clean up inventory
			iObj.Status.PXEBootInterface = seederv1alpha1.PXEBootInterface{}
			iObj.Status.Cluster = seederv1alpha1.ObjectReference{}
			iObj.Status.GeneratedPassword = ""
			iObj.Status.PowerAction.LastJobName = ""
			util.RemoveCondition(iObj, seederv1alpha1.InventoryAllocatedToCluster)
			util.RemoveCondition(iObj, seederv1alpha1.TinkHardwareCreated)
			util.RemoveCondition(iObj, seederv1alpha1.HarvesterJoinNode)
			util.RemoveCondition(iObj, seederv1alpha1.TinkWorkflowCreated)
			util.RemoveCondition(iObj, seederv1alpha1.TinkTemplateCreated)
			util.RemoveCondition(iObj, seederv1alpha1.ClusterCleanupSubmitted)
			util.CreateOrUpdateCondition(iObj, seederv1alpha1.InventoryFreed, "")
			if err := r.Status().Update(ctx, iObj); err != nil {
				return err
			}

			var notFound bool
			// find and clean up hardware object
			hw := &tinkv1alpha1.Hardware{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: iObj.Namespace, Name: iObj.Name}, hw); err != nil {
				if apierrors.IsNotFound(err) {
					notFound = true
				} else {
					return err
				}
			}

			if !notFound {
				if err := r.Delete(ctx, hw); err != nil {
					return err
				}
			}

			// find and clean up template object
			template := &tinkv1alpha1.Template{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: iObj.Namespace, Name: iObj.Name}, template); err != nil {
				if apierrors.IsNotFound(err) {
					notFound = true
				} else {
					return err
				}
			}

			if !notFound {
				if err := r.Delete(ctx, template); err != nil {
					return err
				}
			}

			// find and clean up workflow object
			workflow := &tinkv1alpha1.Workflow{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: iObj.Namespace, Name: iObj.Name}, workflow); err != nil {
				if apierrors.IsNotFound(err) {
					notFound = true
				} else {
					return err
				}
			}

			if !notFound {
				if err := r.Delete(ctx, workflow); err != nil {
					return err
				}
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
			if !util.ConditionExists(iObj, seederv1alpha1.InventoryAllocatedToCluster) || iObj.Status.Cluster.Namespace != c.Namespace || iObj.Status.Cluster.Name != c.Name {
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
func (r *ClusterReconciler) cleanupClusterDeps(ctx context.Context, cObj *seederv1alpha1.Cluster) error {
	r.Info("cleaning up cluster components", "cluster", cObj.Name)
	c := cObj.DeepCopy()
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

		if !inventorymissing {
			// make sure inventory is actually allocated to current cluster. This is a minor change since we now apply a finalizer to cluster at start of reconcile loop. This will result in the deletion reconcile getting triggered.
			// there may be cases where an inventory has been accidentally allocated to a cluster so it never gets patched, so we need to ensure it does not get shutdown
			if i.Status.Cluster.Name != c.Name || i.Status.Cluster.Namespace != c.Namespace {
				continue
			}

			if util.ConditionExists(i, seederv1alpha1.BMCJobSubmitted) {
				return fmt.Errorf("waiting for existing bmcjob to be reconcilled from inventory %s before triggering cleanup", i.Name)
			}

			ok, err := r.ensureInventoryIsShutdown(ctx, c, i)
			if err != nil {
				return fmt.Errorf("error ensuring inventory %s is shutdown %v", i.Name, err)
			}

			if !ok {
				return fmt.Errorf("waiting for inventory %s to be shutdown", i.Name)
			}
			// fetch and clear last job request
			iObj := &seederv1alpha1.Inventory{}
			err = r.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, iObj)
			if err != nil {
				return fmt.Errorf("error getting inventory during cleanupClusterDeps %s %v", i.Name, err)
			}
			iObj.Spec.PowerActionRequested = ""
			err = r.Update(ctx, iObj)
			if err != nil {
				return fmt.Errorf("error resetting powerActionRequested on inventory %s during cleanupClusterDeps %v", i.Name, err)
			}
			iObj.Status.PXEBootInterface = seederv1alpha1.PXEBootInterface{}
			iObj.Status.Cluster = seederv1alpha1.ObjectReference{}
			iObj.Status.GeneratedPassword = ""
			iObj.Status.PowerAction.LastJobName = ""
			util.RemoveCondition(iObj, seederv1alpha1.InventoryAllocatedToCluster)
			util.RemoveCondition(iObj, seederv1alpha1.HarvesterJoinNode)
			util.RemoveCondition(iObj, seederv1alpha1.HarvesterCreateNode)
			util.RemoveCondition(iObj, seederv1alpha1.ClusterCleanupSubmitted)
			util.CreateOrUpdateCondition(i, seederv1alpha1.InventoryFreed, "")
			err = r.Status().Update(ctx, iObj)

			if err != nil {
				return err
			}
		}

		if !poolmissing {
			delete(pool.Status.AddressAllocation, i.Status.PXEBootInterface.Address)
			if err := r.lockedAddressPoolUpdate(ctx, pool); err != nil {
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
			if err := r.lockedAddressPoolUpdate(ctx, pool); err != nil {
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
func (r *ClusterReconciler) markClusterReady(ctx context.Context, cObj *seederv1alpha1.Cluster) error {
	c := cObj.DeepCopy()
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

	if len(nl.Items) != len(c.Spec.Nodes) {
		return fmt.Errorf("api server is running, expected to find %d nodes but only found %d nodes", len(nl.Items), len(c.Spec.Nodes))
	}

	c.Status.Status = seederv1alpha1.ClusterRunning
	return r.Status().Update(ctx, c)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&seederv1alpha1.Cluster{}).
		Watches(&tinkv1alpha1.Hardware{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, a client.Object) []reconcile.Request {
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
		})).Named("cluster").
		Complete(r)
}

func genCoreTypedClient(ctx context.Context, c *seederv1alpha1.Cluster) (*typedCore.CoreV1Client, error) {
	port, ok := c.Labels[seederv1alpha1.OverrideAPIPortLabel]
	if !ok {
		port = seederv1alpha1.DefaultAPIPort
	}

	var isLocalCluster bool
	var restConfig *rest.Config
	if c.Name == seederv1alpha1.DefaultLocalClusterName && c.Namespace == seederv1alpha1.DefaultLocalClusterNamespace {
		isLocalCluster = true
	}

	// special handling for local cluster to use InClusterConfig
	if isLocalCluster {
		var err error
		restConfig, err = ctrl.GetConfig()
		if err != nil {
			return nil, fmt.Errorf("error fetching incluster config: %v", err)
		}
	} else {
		kcBytes, err := util.GenerateKubeConfig(c.Status.ClusterAddress, port, seederv1alpha1.DefaultAPIPrefix, c.Status.ClusterToken)
		if err != nil {
			return nil, err
		}

		hcClientConfig, err := clientcmd.NewClientConfigFromBytes(kcBytes)
		if err != nil {
			return nil, err
		}

		restConfig, err = hcClientConfig.ClientConfig()
		if err != nil {
			return nil, err
		}
	}

	return typedCore.NewForConfig(restConfig)
}

func createOrUpdateInventoryConditions(ctx context.Context, inventory *seederv1alpha1.Inventory, cond condition.Cond, msg string, client client.Client) error {
	iObj := &seederv1alpha1.Inventory{}
	if err := client.Get(ctx, types.NamespacedName{Name: inventory.Name, Namespace: inventory.Namespace}, iObj); err != nil {
		return err
	}

	if !util.ConditionExists(iObj, cond) {
		util.CreateOrUpdateCondition(iObj, cond, msg)
		if err := client.Status().Update(ctx, iObj); err != nil {
			return err
		}
	}

	return nil
}

func (r *ClusterReconciler) createOrUpdateHardware(ctx context.Context, hardware *tinkv1alpha1.Hardware, inventory *seederv1alpha1.Inventory) error {
	hardwareObj := &tinkv1alpha1.Hardware{}
	err := r.Get(ctx, types.NamespacedName{Name: hardware.Name, Namespace: hardware.Namespace}, hardwareObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if createErr := r.Create(ctx, hardware); createErr != nil {
				return createErr
			}
		}
		return err
	}

	return createOrUpdateInventoryConditions(ctx, inventory, seederv1alpha1.TinkHardwareCreated, "tink hardware created", r.Client)
}

// ensureInventoryIsShutdown ensures underlying Machine is shutdown before inventory is freed from cluster
func (r *ClusterReconciler) ensureInventoryIsShutdown(ctx context.Context, c *seederv1alpha1.Cluster, i *seederv1alpha1.Inventory) (bool, error) {
	r.Info("ensuring inventory is shutdown", "inventory", i.Name)
	machineObj := &rufio.Machine{}
	err := r.Get(ctx, types.NamespacedName{Name: i.Name, Namespace: i.Namespace}, machineObj)
	if err != nil {
		return false, fmt.Errorf("error fetching machine %s: %w", machineObj.Name, err)
	}

	// machine is already turned off, nothing else needed
	if machineObj.Status.Power == rufio.Off {
		return true, nil
	}

	if !util.ConditionExists(i, seederv1alpha1.ClusterCleanupSubmitted) {
		iObj := i.DeepCopy()
		util.CreateOrUpdateCondition(i, seederv1alpha1.ClusterCleanupSubmitted, c.Name)
		i.Status.PowerAction.LastJobName = ""
		err := r.Status().Patch(ctx, i, client.MergeFrom(iObj))
		if err != nil {
			return false, fmt.Errorf("error patching inventory status while triggering shutdown: %w", err)
		}
		// fetch i and apply conditionth
		err = r.Get(ctx, types.NamespacedName{Name: iObj.Name, Namespace: iObj.Namespace}, i)
		if err != nil {
			return false, fmt.Errorf("error fetching inventory %s: %w", iObj.Name, err)
		}

		iObj = i.DeepCopy()

		i.Spec.PowerActionRequested = seederv1alpha1.NodePowerActionShutdown

		// return false and update from status patch
		// on subsequent reconcile with condition is found
		// we validate machine state
		return false, r.Patch(ctx, i, client.MergeFrom(iObj))
	}

	// patch machineObj with an annotation to trigger reconcile
	// else the machine reconcile will occur every 3 mins which can cause
	// cluster deletion to be blocked
	machineObjCopy := machineObj.DeepCopy()
	if machineObj.Annotations == nil {
		machineObj.Annotations = map[string]string{}
	}
	machineObj.Annotations[seederv1alpha1.MachineReconcileAnnotationName] = metav1.Now().String()
	return false, r.Patch(ctx, machineObj, client.MergeFrom(machineObjCopy))
}

// lockedAddressPool ensures only one caller can perform an update at a time
func (r *ClusterReconciler) lockedAddressPoolUpdate(ctx context.Context, pool *seederv1alpha1.AddressPool) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.Status().Update(ctx, pool)
}
