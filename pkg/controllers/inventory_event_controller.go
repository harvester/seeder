package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/harvester/seeder/pkg/events"
)

// InventoryEventReconciler reconciles events for an inventory object
type InventoryEventReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logr.Logger
	record.EventRecorder
}

const (
	NextCheckTime = "nextCheckTime"
)

type inventoryEventReconciler func(context.Context, *seederv1alpha1.Inventory) error

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Cluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *InventoryEventReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Info("Reconcilling inventory objects for events", req.Name, req.Namespace)
	// TODO(user): your logic here
	i := &seederv1alpha1.Inventory{}

	err := r.Get(ctx, req.NamespacedName, i)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.Error(err, "unable to fetch inventory object")
		return ctrl.Result{}, err
	}

	// if Event lookup is disabled, ignore the objects
	if !i.Spec.Events.Enabled {
		return ctrl.Result{}, nil
	}

	// if inventory is not ready, then return and wait for it to be ready
	if i.Status.Status != seederv1alpha1.InventoryReady {
		return ctrl.Result{}, fmt.Errorf("waiting for inventory %s in namespace %s to be ready", i.Name, i.Namespace)
	}

	// if next check time is after current time then requeue
	timeStamp, ok := i.Annotations[NextCheckTime]
	if ok {
		parsedTime, err := time.Parse(time.RFC3339, timeStamp)
		if err != nil {
			return ctrl.Result{}, err
		}

		if parsedTime.After(time.Now()) {
			time.Until(parsedTime)
			return ctrl.Result{RequeueAfter: time.Until(parsedTime)}, nil
		}

	}

	reconcileList := []inventoryEventReconciler{
		r.getInventoryInfo,
	}

	if i.DeletionTimestamp.IsZero() {
		for _, reconciler := range reconcileList {
			if err := reconciler(ctx, i); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{RequeueAfter: time.Duration(1 * time.Hour)}, nil
}

// getInventoryInfo will leverage Redfish to query inventory information
func (r *InventoryEventReconciler) getInventoryInfo(ctx context.Context, i *seederv1alpha1.Inventory) error {

	// next reconcile duration
	duration, err := time.ParseDuration(i.Spec.Events.PollingInterval)
	if err != nil {
		return err
	}
	// fetch bmc secret first
	s := &corev1.Secret{}
	err = r.Client.Get(ctx, types.NamespacedName{Namespace: i.Spec.BaseboardManagementSpec.Connection.AuthSecretRef.Namespace,
		Name: i.Spec.BaseboardManagementSpec.Connection.AuthSecretRef.Name}, s)

	if err != nil {
		return err
	}

	username, ok := s.Data["username"]
	if !ok {
		return fmt.Errorf("secret %s has no key username", s.Name)
	}
	password, ok := s.Data["password"]
	if !ok {
		return fmt.Errorf("secret %s has no key password", s.Name)
	}

	bmcendpoint := fmt.Sprintf("https://%s", i.Spec.BaseboardManagementSpec.Connection.Host)
	if port, ok := i.Labels[seederv1alpha1.OverrideRedfishPortLabel]; ok {
		bmcendpoint = fmt.Sprintf("https://%s:%s", i.Spec.BaseboardManagementSpec.Connection.Host, port)
	}
	rc, err := events.NewEventFetcher(ctx, string(username), string(password), bmcendpoint)
	if err != nil {
		return err
	}

	labels, status, err := rc.GetConfig()
	if err != nil {
		return err
	}

	// record event and update object
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		obj := &seederv1alpha1.Inventory{}
		err := r.Get(ctx, types.NamespacedName{Namespace: i.Namespace, Name: i.Name}, obj)
		if err != nil {
			return err
		}

		if obj.Labels == nil {
			obj.Labels = make(map[string]string)
		}

		if obj.Annotations == nil {
			obj.Annotations = make(map[string]string)
		}

		obj.Annotations[NextCheckTime] = time.Now().Add(duration).Format(time.RFC3339)
		for k, v := range labels {
			obj.Labels[k] = v
		}
		return r.Update(ctx, obj)
	})

	if err != nil {
		return err
	}

	for _, v := range status {
		r.EventRecorder.Event(i, "Normal", "RedfishStatusEvent", fmt.Sprintf("current inventory status: %s", v))
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *InventoryEventReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&seederv1alpha1.Inventory{}).
		Complete(r)
}
