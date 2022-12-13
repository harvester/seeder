package controllers

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-logr/logr"
	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type localNodeReconciler func(context.Context, *corev1.Node) error

type LocalNodeReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logr.Logger
	record.EventRecorder
}

// Reconcile nodes in the current cluster to watch for updates for power cycle annotations
func (r *LocalNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Info("Reconcilling node in local cluster", req.Name, req.Namespace)

	n := &corev1.Node{}

	err := r.Get(ctx, req.NamespacedName, n)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.Error(err, "unable to fetch inventory object")
		return ctrl.Result{}, err
	}

	reconcileList := []localNodeReconciler{
		r.manageNodeActions,
	}

	if n.DeletionTimestamp.IsZero() {
		for _, reconciler := range reconcileList {
			if err := reconciler(ctx, n); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// manageNodeActions will manage node actions and process cleanup of completed jobs
func (r *LocalNodeReconciler) manageNodeActions(ctx context.Context, node *corev1.Node) error {
	_, ok := node.Annotations[seederv1alpha1.NodeActionRequested]
	if !ok {
		r.Info("skipping node object as no action is requested", node.Name, node.Namespace)
		return nil
	}

	// clean up old job if present
	_, jobCompletedOK := node.Annotations[seederv1alpha1.NodeActionStatus]
	if jobCompletedOK {
		return r.cleanupJob(ctx, node)
	}
	return r.powerAction(ctx, node)
}

// powerAction reconciles the nodes until the seederv1alpha1.NodeActionRequested exists on the node.
// once the power action has been completed then the annotation is removed, and node is ignored
func (r *LocalNodeReconciler) powerAction(ctx context.Context, node *corev1.Node) error {
	existingJobName, ok := node.Annotations[seederv1alpha1.NodePowerActionJobName]
	if !ok {
		powerAction := node.Annotations[seederv1alpha1.NodeActionRequested]
		jobName := generateJobName(*node)
		if err := r.checkAndCreateJob(ctx, node, powerAction); err != nil {
			return err
		}

		additionalAnnotations := map[string]string{
			seederv1alpha1.NodePowerActionJobName: jobName,
			seederv1alpha1.NodeLastActionRequest:  powerAction,
		}

		// remove previous job status annotation if any are present
		removeAnootations := []string{
			seederv1alpha1.NodeActionStatus,
		}
		return r.nodeUpdateHelper(ctx, node, additionalAnnotations, removeAnootations)
	}

	// check job status and wait for completion
	jobObj := &rufio.Job{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: seederv1alpha1.DefaultLocalClusterNamespace, Name: existingJobName}, jobObj); err != nil {
		return fmt.Errorf("error fetching job during status check for job %s: %v", existingJobName, err)
	}

	addAnnotations := make(map[string]string)
	toRemove := []string{
		seederv1alpha1.NodeActionRequested,
	}
	// check if job is complete or failed
	if jobObj.HasCondition(rufio.JobCompleted, rufio.ConditionTrue) {
		addAnnotations[seederv1alpha1.NodeActionStatus] = seederv1alpha1.NodeJobComplete
	} else if jobObj.HasCondition(rufio.JobFailed, rufio.ConditionTrue) {
		addAnnotations[seederv1alpha1.NodeActionStatus] = seederv1alpha1.NodeJobFailed
	} else if jobObj.HasCondition(rufio.JobRunning, rufio.ConditionTrue) {
		return fmt.Errorf("job %s for node %s still running", jobObj.Name, node.Name)
	} else {
		return fmt.Errorf("waiting for conditions to be set on job %s in namespace %s", jobObj.Name, jobObj.Namespace)
	}

	return r.nodeUpdateHelper(ctx, node, addAnnotations, toRemove)
}

func (r *LocalNodeReconciler) checkAndCreateJob(ctx context.Context, node *corev1.Node, powerAction string) error {
	jobObj := &rufio.Job{}
	jobName := generateJobName(*node)
	var notFound bool

	err := r.Get(ctx, types.NamespacedName{Namespace: seederv1alpha1.DefaultLocalClusterNamespace, Name: jobName}, jobObj)

	if err != nil {
		if apierrors.IsNotFound(err) {
			notFound = true
		} else {
			return fmt.Errorf("error fetching job object %s: %v", jobName, err)
		}
	}

	if notFound {
		newJob := generateJob(node.Name, powerAction)
		err = controllerutil.SetOwnerReference(node, newJob, r.Scheme)
		if err != nil {
			return fmt.Errorf("error setting owner reference on job %s: %v", jobName, err)
		}
		if err := r.Create(ctx, newJob); err != nil {
			return fmt.Errorf("error creating power action job for node %s: %v", jobName, err)
		}
	}

	node.Annotations[seederv1alpha1.NodePowerActionJobName] = jobName
	return nil
}

func generateJob(nodeName, powerAction string) *rufio.Job {
	var tasks []rufio.Action
	powerOffTask := rufio.Action{
		PowerAction: rufio.PowerHardOff.Ptr(),
	}
	powerOnTask := rufio.Action{
		PowerAction: rufio.PowerOn.Ptr(),
	}

	switch powerAction {
	case seederv1alpha1.NodePowerActionPowerOn:
		tasks = append(tasks, powerOnTask)
	case seederv1alpha1.NodePowerActionShutdown:
		tasks = append(tasks, powerOffTask)
	case seederv1alpha1.NodePowerActionReboot:
		tasks = append(tasks, powerOffTask, powerOnTask)
	}

	return &rufio.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", nodeName, powerAction),
			Namespace: seederv1alpha1.DefaultLocalClusterNamespace,
			Labels: map[string]string{
				"inventory": nodeName,
			},
		},
		Spec: rufio.JobSpec{
			MachineRef: rufio.MachineRef{
				Name:      nodeName,
				Namespace: seederv1alpha1.DefaultLocalClusterNamespace,
			},
			Tasks: tasks,
		},
	}
}

func generateJobName(node corev1.Node) string {
	powerAction := node.Annotations[seederv1alpha1.NodeActionRequested]
	return fmt.Sprintf("%s-%s", node.Name, powerAction)
}

// cleanupJob cleans up failed or completed jobs to keep things tidy
func (r *LocalNodeReconciler) cleanupJob(ctx context.Context, node *corev1.Node) error {
	_, jobCompleted := node.Annotations[seederv1alpha1.NodeActionStatus]
	if !jobCompleted {
		return nil
	}

	toRemove := []string{
		seederv1alpha1.NodeActionStatus,
		seederv1alpha1.NodePowerActionJobName,
	}

	jobName, ok := node.Annotations[seederv1alpha1.NodePowerActionJobName]
	if !ok { // no action needed just clean up nodes
		return r.nodeUpdateHelper(ctx, node, nil, toRemove)
	}

	job := &rufio.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: seederv1alpha1.DefaultLocalClusterNamespace}, job)
	if err != nil {
		if apierrors.IsNotFound(err) { // job not found. clean up node annotations
			return r.nodeUpdateHelper(ctx, node, nil, toRemove)
		} else {
			return err
		}
	}

	// remove job and update annotations
	if job.HasCondition(rufio.JobCompleted, rufio.ConditionTrue) || job.HasCondition(rufio.JobFailed, rufio.ConditionTrue) {
		if err := r.Delete(ctx, job); err != nil {
			return err
		}

		r.EventRecorder.Event(node, "Normal", "PowerActionJobCleanup", fmt.Sprintf("job %s in namespace %s removed", jobName, seederv1alpha1.DefaultLocalClusterNamespace))
	}

	return r.nodeUpdateHelper(ctx, node, nil, toRemove)
}

// nodeUpdateHelper is a generic function that allows annotations to be added / removed while using retryOnConflict
func (r *LocalNodeReconciler) nodeUpdateHelper(ctx context.Context, node *corev1.Node, addAnnotations map[string]string, removeAnnotations []string) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		obj := &corev1.Node{}
		err := r.Get(ctx, types.NamespacedName{Namespace: node.Namespace, Name: node.Name}, obj)
		if err != nil {
			return err
		}

		if obj.Annotations == nil {
			obj.Annotations = make(map[string]string)
		}

		// keys to be added
		for k, v := range addAnnotations {
			obj.Annotations[k] = v
		}

		// keys to be removed
		for _, v := range removeAnnotations {
			delete(obj.Annotations, v)
		}

		return r.Update(ctx, obj)
	})
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *LocalNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		Complete(r)
}
