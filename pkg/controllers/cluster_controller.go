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
	bmaasv1alpha1 "github.com/harvester/bmaas/pkg/api/v1alpha1"
	"github.com/harvester/bmaas/pkg/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logr.Logger
}

type clusterReconciler func(context.Context, *bmaasv1alpha1.Cluster) error

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
	c := &bmaasv1alpha1.Cluster{}

	err := r.Get(ctx, req.NamespacedName, c)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.Error(err, "unable to fetch cluster object")
		return ctrl.Result{}, err
	}

	reconcileList := []clusterReconciler{
		r.generateClusterConfig,
		r.patchNodesAndPools,
	}
	deletionReconcileList := []clusterReconciler{}

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
func (r *ClusterReconciler) generateClusterConfig(ctx context.Context, c *bmaasv1alpha1.Cluster) error {
	if c.Status.Status == "" {
		vipPool := &bmaasv1alpha1.AddressPool{}
		err := r.Get(ctx, types.NamespacedName{Namespace: c.Spec.VIPConfig.AddressPoolReference.Namespace,
			Name: c.Spec.VIPConfig.AddressPoolReference.Name}, vipPool)
		if err != nil {
			return err
		}

		vip, err := util.AllocateAddress(vipPool.Status.DeepCopy(), c.Spec.VIPConfig.StaticAddress)
		if err != nil {
			return err
		}

		c.Status.ClusterAddress = vip
		c.Status.ClusterToken = util.GenerateRand()
		c.Status.Status = bmaasv1alpha1.ClusterConfigReady
		return r.Status().Update(ctx, c)
	}
	return nil
}

// patchNodes will patch the node information and associate appropriate events to trigger
// tinkerbell workflows to be generated and reboot initiated
func (r *ClusterReconciler) patchNodesAndPools(ctx context.Context, c *bmaasv1alpha1.Cluster) error {
	if c.Status.Status == bmaasv1alpha1.ClusterConfigReady {
		for n, nc := range c.Spec.Nodes {
			pool := &bmaasv1alpha1.AddressPool{}
			err := r.Get(ctx, types.NamespacedName{Namespace: nc.AddressPoolReference.Name,
				Name: nc.AddressPoolReference.Name}, pool)
			if err != nil {
				return err
			}
			i := &bmaasv1alpha1.Inventory{}
			err = r.Get(ctx, types.NamespacedName{Namespace: nc.InventoryReference.Name,
				Name: nc.InventoryReference.Name}, i)
			if err != nil {
				return err
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
				nodeAddress, err = util.AllocateAddress(pool.Status.DeepCopy(), nc.StaticAddress)
			}

			if err != nil {
				return err
			}

			i.Status.PXEBootInterface.Address = nodeAddress
			i.Status.PXEBootInterface.Gateway = pool.Spec.Gateway
			i.Status.PXEBootInterface.Netmask = pool.Status.Netmask

			// node password and conditions
			i.Status.GeneratedPassword = util.GenerateRand()
			i.Status.Cluster.Namespace = c.Namespace
			i.Status.Cluster.Name = c.Name
			i.Status.Conditions = util.CreateOrUpdateCondition(i.Status.Conditions, bmaasv1alpha1.InventoryAllocatedToCluster,
				fmt.Sprintf("node assigned to cluster %s", c.Name))
			i.Status.Conditions = util.RemoveCondition(i.Status.Conditions, bmaasv1alpha1.InventoryFreed)

			if n == 0 {
				i.Status.Conditions = util.CreateOrUpdateCondition(i.Status.Conditions, bmaasv1alpha1.HarvesterCreateNode, "Create Mode")
			} else {
				i.Status.Conditions = util.CreateOrUpdateCondition(i.Status.Conditions, bmaasv1alpha1.HarvesterJoinNode, "Join Mode")
			}

			err = r.Status().Update(ctx, i)
			if err != nil {
				return err
			}
			// update pool with node allocation if not already done
			if !found {
				pool.Status.AddressAllocation[nodeAddress] = bmaasv1alpha1.ObjectReference{
					Namespace: i.Namespace,
					Name:      i.Name,
				}
				err = r.Status().Update(ctx, pool)
				if err != nil {
					return err
				}
			}
		}

		c.Status.Status = bmaasv1alpha1.ClusterNodesPatched
		err := r.Status().Update(ctx, c)
		if err != nil {
			return err
		}

		if !controllerutil.ContainsFinalizer(c, bmaasv1alpha1.ClusterFinalizer) {
			controllerutil.AddFinalizer(c, bmaasv1alpha1.ClusterFinalizer)
			return r.Update(ctx, c)
		}

	}
	return nil
}

// cleanupClusterDeps will trigger cleanup of nodes and associated infra
func (r *ClusterReconciler) cleanupClusterDeps(ctx context.Context, c *bmaasv1alpha1.Cluster) error {
	for _, nc := range c.Spec.Nodes {
		pool := &bmaasv1alpha1.AddressPool{}
		err := r.Get(ctx, types.NamespacedName{Namespace: nc.AddressPoolReference.Name,
			Name: nc.AddressPoolReference.Name}, pool)
		if err != nil {
			return err
		}
		i := &bmaasv1alpha1.Inventory{}
		err = r.Get(ctx, types.NamespacedName{Namespace: nc.InventoryReference.Name,
			Name: nc.InventoryReference.Name}, i)
		if err != nil {
			return err
		}

		delete(pool.Status.AddressAllocation, i.Status.PXEBootInterface.Address)

		i.Status.PXEBootInterface = bmaasv1alpha1.PXEBootInterface{}
		i.Status.Cluster = bmaasv1alpha1.ObjectReference{}
		i.Status.GeneratedPassword = ""
		i.Status.Conditions = util.RemoveCondition(i.Status.Conditions, bmaasv1alpha1.InventoryAllocatedToCluster)
		i.Status.Conditions = util.CreateOrUpdateCondition(i.Status.Conditions, bmaasv1alpha1.InventoryFreed, "")
		i.Status.Conditions = util.RemoveCondition(i.Status.Conditions, bmaasv1alpha1.HarvesterCreateNode)
		i.Status.Conditions = util.RemoveCondition(i.Status.Conditions, bmaasv1alpha1.HarvesterJoinNode)
		err = r.Status().Update(ctx, i)
		if err != nil {
			return err
		}
		return r.Status().Update(ctx, pool)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bmaasv1alpha1.Cluster{}).
		Complete(r)
}
