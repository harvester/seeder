package controllers

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ClusterEventReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logr.Logger
	record.EventRecorder
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
		r.updateHardware,
		r.hardwareEvents,
	}

	for _, reconciler := range reconcileList {
		if err := reconciler(ctx, c); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: 1 * time.Hour}, nil
}

func (r *ClusterEventReconciler) updateHardware(ctx context.Context, c *seederv1alpha1.Cluster) error {
	return nil
}

func (r *ClusterEventReconciler) hardwareEvents(ctx context.Context, c *seederv1alpha1.Cluster) error {
	return nil
}
