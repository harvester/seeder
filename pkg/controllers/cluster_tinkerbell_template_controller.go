package controllers

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/tink"
	"github.com/harvester/seeder/pkg/util"
)

// ClusterTinkerbellTemplateReconciler reconciles a Cluster object and watches associated TinkerbellTemplate for changes
type ClusterTinkerbellTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logr.Logger
}

func (r *ClusterTinkerbellTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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
		r.createTinkerbellTemplate,
	}

	if c.DeletionTimestamp.IsZero() {
		for _, reconciler := range reconcileList {
			if err := reconciler(ctx, c); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// createTinkerbellTemplate will create template objects for all nodes in the cluster
func (r *ClusterTinkerbellTemplateReconciler) createTinkerbellTemplate(ctx context.Context, cObj *seederv1alpha1.Cluster) error {

	c := cObj.DeepCopy()
	if c.Status.Status == seederv1alpha1.ClusterNodesPatched || c.Status.Status == seederv1alpha1.ClusterTinkHardwareSubmitted || c.Status.Status == seederv1alpha1.ClusterRunning {
		// check to see if the service for tink-stack is ready
		tinkStackService := &corev1.Service{}
		err := r.Get(ctx, types.NamespacedName{Name: seederv1alpha1.DefaultTinkStackService, Namespace: namespace}, tinkStackService)
		if err != nil {
			return fmt.Errorf("error fetching svc %s in ns %s: %v", seederv1alpha1.DefaultTinkStackService, c.Namespace, err)
		}

		seederConfig := &corev1.ConfigMap{}
		err = r.Get(ctx, types.NamespacedName{Name: seederv1alpha1.SeederConfig, Namespace: c.Namespace}, seederConfig)
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("error fetching configmap %s in ns %s: %v", seederv1alpha1.SeederConfig, c.Namespace, err)
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

			template, err := tink.GenerateTemplate(tinkStackService, seederConfig, inventory, c)
			if err != nil {
				return err
			}
			// create hardware object
			err = controllerutil.SetOwnerReference(c, template, r.Scheme)
			if err != nil {
				return err
			}

			// create / update hardware object if one already exists
			err = r.createOrUpdateTemplate(ctx, template, inventory)
			if err != nil {
				return err
			}
		}

	}

	return nil
}

// createOrUpdateTemplate will create or update the tinkerbell Template
func (r *ClusterTinkerbellTemplateReconciler) createOrUpdateTemplate(ctx context.Context, template *tinkv1alpha1.Template, inventory *seederv1alpha1.Inventory) error {
	templateObj := &tinkv1alpha1.Template{}
	err := r.Get(ctx, types.NamespacedName{Name: template.Name, Namespace: template.Namespace}, templateObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if createErr := r.Create(ctx, template); createErr != nil {
				return createErr
			}
		}
		return err
	}

	if !reflect.DeepEqual(templateObj.Spec, template.Spec) {
		templateObj.Spec = template.Spec
		if err := r.Update(ctx, templateObj); err != nil {
			return err
		}
	}

	return createOrUpdateInventoryConditions(ctx, inventory, seederv1alpha1.TinkTemplateCreated, "tink template created", r.Client)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterTinkerbellTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&seederv1alpha1.Cluster{}).
		Watches(&tinkv1alpha1.Template{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, a client.Object) []reconcile.Request {
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
