package mock

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ignoreMachineNamePrefix = "ignore-"
)

// FakeBaseboardJobReconciller implements a fake reconcile loop for integration testing
type FakeBaseboardJobReconciller struct {
	client.Client
	logr.Logger
	Scheme *runtime.Scheme
}

func (f *FakeBaseboardJobReconciller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	jobObj := &rufio.Job{}
	f.Info("Reconcilling baseboard job objects", req.Name, req.Namespace)
	err := f.Get(ctx, req.NamespacedName, jobObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		f.Error(err, "error fetching baseboard job object")
		return ctrl.Result{}, err
	}

	// skip jobs which have completed
	if jobObj.HasCondition(rufio.JobCompleted, rufio.ConditionTrue) || jobObj.DeletionTimestamp != nil {
		f.Info("skipping job as it has completed", jobObj.Name, jobObj.Namespace)
		return ctrl.Result{}, nil
	}

	jobObjCopy := jobObj.DeepCopy()
	if err := f.PatchMachinePowerStatus(ctx, jobObjCopy); err != nil {
		return ctrl.Result{}, err
	}
	jobObj.Status = rufio.JobStatus{}
	// patch power status to mimic real world action
	jobObj.SetCondition(rufio.JobCompleted, rufio.ConditionTrue)
	jobObj.SetCondition(rufio.JobRunning, rufio.ConditionTrue)
	currentTime := metav1.Now()
	jobObj.Status.StartTime = &currentTime
	jobObj.Status.CompletionTime = &currentTime

	err = f.Status().Patch(ctx, jobObj, client.MergeFrom(jobObjCopy))
	if err != nil {
		f.Error(err, "error during status patch", jobObj.Name, jobObj.Namespace)
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (f *FakeBaseboardJobReconciller) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rufio.Job{}).Named("fakebmcjob").
		Complete(f)
}

// PatchMachinePowerStatus will update the machine objects power status to match the last requested action in the job
func (f *FakeBaseboardJobReconciller) PatchMachinePowerStatus(ctx context.Context, j *rufio.Job) error {
	machineObj := &rufio.Machine{}
	err := f.Get(ctx, types.NamespacedName{Name: j.Spec.MachineRef.Name, Namespace: j.Spec.MachineRef.Namespace}, machineObj)
	if err != nil {
		return fmt.Errorf("error fetching machine %s/%s: %w", j.Spec.MachineRef.Namespace, j.Spec.MachineRef.Name, err)
	}

	// skip machines with ignoreMachineNamnePrefix to help simulate when machine is not powered off
	// and this can be used to ensure a second poweroff job is submitted by cluster controller
	updatedMachine, err := f.skipMachinePoweroffUntilMultipleJobsExist(ctx, machineObj)
	if err != nil {
		return fmt.Errorf("error during method skipMachinePoweroffUntilMultipleJobsExist: %v", err)
	}

	//  do not perform machine power status updated yet
	if !updatedMachine {
		return nil
	}

	lastAction := j.Spec.Tasks[len(j.Spec.Tasks)-1]
	machineObj.Status.Power = rufio.PowerState(*lastAction.PowerAction)
	err = f.Status().Update(ctx, machineObj)
	if err != nil {
		return err
	}
	return nil
}

func (f *FakeBaseboardJobReconciller) skipMachinePoweroffUntilMultipleJobsExist(ctx context.Context, machine *rufio.Machine) (bool, error) {
	if !strings.Contains(machine.Name, ignoreMachineNamePrefix) {
		// no further action needed, exit function and allow power status to be updated
		return true, nil
	}
	jobList := &rufio.JobList{}
	err := f.List(ctx, jobList, client.InNamespace(machine.Namespace))
	if err != nil {
		return false, err
	}

	shutdownJobCount := 0
	// found more than 1 job
	for _, v := range jobList.Items {
		// there will be a reboot job associated from initial provisioning
		// so we need to filter out for shutdown jobs only
		if strings.Contains(v.Name, "shutdown") && strings.Contains(v.Name, machine.Name) {
			shutdownJobCount++
		}
	}

	if shutdownJobCount > 2 {
		return true, nil
	}
	return false, nil
}
