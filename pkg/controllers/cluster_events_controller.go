package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	typedCore "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/events"
)

type ClusterEventReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logr.Logger
	record.EventRecorder
}

type nodeLabelsPatch struct {
	Metadata struct {
		Labels map[string]string `json:"labels,omitempty"`
	} `json:"metadata"`
}

type clusterEventReconciler func(context.Context, *seederv1alpha1.Cluster) error

func (r *ClusterEventReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Info("Reconcilling cluster objects for events", req.Name, req.Namespace)
	// TODO(user): your logic here
	c := &seederv1alpha1.Cluster{}

	err := r.Get(ctx, req.NamespacedName, c)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.Error(err, "unable to fetch cluster object")
		return ctrl.Result{}, err
	}

	// ignore cluster deletion
	if !c.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// if cluster is not yet running then ignore the cluster
	if c.Status.Status != seederv1alpha1.ClusterRunning {
		return ctrl.Result{}, nil
	}

	reconcileList := []clusterEventReconciler{
		r.updateNodes,
	}

	for _, reconciler := range reconcileList {
		if err := reconciler(ctx, c); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: 15 * time.Minute}, nil
}

func (r *ClusterEventReconciler) updateNodes(ctx context.Context, c *seederv1alpha1.Cluster) error {
	typedClient, err := genCoreTypedClient(ctx, c)
	if err != nil {
		return err
	}

	inventoryList, err := r.identifyInventory(ctx, c)
	if err != nil {
		return err
	}

	if len(inventoryList) == 0 {
		// no nodes have event collection enabled. nothing to do
		return nil
	}

	nodeList, err := typedClient.Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	// associate k8s node with inventory using the address allocated to inventory by cluster
	// this should make it easy to uniquely identify nodes in the cluster
	var errs []error
	var ctxErr error
	for _, i := range inventoryList {
		if ctxErr = ctx.Err(); ctxErr != nil {
			break
		}

		node := findNodeByIP(nodeList.Items, i.Status.Address)
		if node == nil {
			continue
		}

		if err := r.updateNode(ctx, typedClient, i, node); err != nil {
			errs = append(errs, fmt.Errorf("inventory %s/%s: %w", i.Namespace, i.Name, err))
		}
	}
	return errors.Join(append(errs, ctxErr)...)
}

func patchNodeLabels(ctx context.Context, typedClient *typedCore.CoreV1Client, name string, labels map[string]string) (*corev1.Node, error) {
	nodeLabels := nodeLabelsPatch{}
	nodeLabels.Metadata.Labels = make(map[string]string)
	maps.Copy(nodeLabels.Metadata.Labels, labels)

	nodeLabelsJSON, err := json.Marshal(nodeLabels)
	if err != nil {
		return nil, err
	}

	return typedClient.Nodes().Patch(ctx, name, types.MergePatchType, nodeLabelsJSON, metav1.PatchOptions{})
}

func (r *ClusterEventReconciler) updateNode(ctx context.Context, typedClient *typedCore.CoreV1Client, inventory *seederv1alpha1.Inventory, node *corev1.Node) error {
	s := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: inventory.Spec.BaseboardManagementSpec.Connection.AuthSecretRef.Namespace,
		Name:      inventory.Spec.BaseboardManagementSpec.Connection.AuthSecretRef.Name,
	}, s)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Error(err, "BMC auth secret not found, skipping event collection for inventory", inventory.Spec.BaseboardManagementSpec.Connection.AuthSecretRef.Name, inventory.Spec.BaseboardManagementSpec.Connection.AuthSecretRef.Namespace)

			return nil
		}
		return err
	}
	username := s.Data["username"]
	password := s.Data["password"]
	bmcendpoint := fmt.Sprintf("https://%s", inventory.Spec.BaseboardManagementSpec.Connection.Host)
	if port, ok := inventory.Labels[seederv1alpha1.OverrideRedfishPortLabel]; ok {
		bmcendpoint = fmt.Sprintf("https://%s:%s", inventory.Spec.BaseboardManagementSpec.Connection.Host, port)
	}
	e, err := events.NewEventFetcher(ctx, string(username), string(password), bmcendpoint)
	if err != nil {
		return err
	}
	defer e.Close()
	labels, status, err := e.GetConfig()
	if err != nil {
		return err
	}

	updatedNode, err := patchNodeLabels(ctx, typedClient, node.Name, labels)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Error(err, "skipping node label update: node not found", "node", node.Name)
			return nil
		}

		return err
	}

	recorder := remoteEventRecorder(typedClient, r.Scheme)
	for _, v := range status {
		update := "Warning"
		recorder.Event(updatedNode, update, seederv1alpha1.EventLoggerName, v)
	}

	return nil
}

func (r *ClusterEventReconciler) identifyInventory(ctx context.Context, c *seederv1alpha1.Cluster) ([]*seederv1alpha1.Inventory, error) {
	var retNodes []*seederv1alpha1.Inventory
	// identify nodes for which event collection is enabled
	for _, v := range c.Spec.Nodes {
		nodeObj := &seederv1alpha1.Inventory{}
		err := r.Get(ctx, types.NamespacedName{Namespace: v.InventoryReference.Namespace, Name: v.InventoryReference.Name}, nodeObj)
		if err != nil {
			return nil, err
		}
		if nodeObj.Spec.Enabled {
			retNodes = append(retNodes, nodeObj)
		}
	}

	return retNodes, nil
}

func findNodeByIP(nodeList []corev1.Node, address string) *corev1.Node {
	for _, v := range nodeList {
		for _, a := range v.Status.Addresses {
			if a.Address == address {
				return &v
			}
		}
	}

	return nil
}

func remoteEventRecorder(c *typedCore.CoreV1Client, scheme *runtime.Scheme) record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartStructuredLogging(0)
	eventBroadcaster.StartRecordingToSink(&typedCore.EventSinkImpl{Interface: c.Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: "boots"})
	return recorder
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterEventReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&seederv1alpha1.Cluster{}).
		Named("clusterevents").
		Complete(r)
}
