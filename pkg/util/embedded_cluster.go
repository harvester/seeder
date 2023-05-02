package util

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

// createLocalCluster is invoked when seeder is run in embedded mode
// and will create a new local cluster, which points to the k8s default svc
func createLocalCluster(ctx context.Context, client client.Client) error {
	localCluster := &seederv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      seederv1alpha1.DefaultLocalClusterName,
			Namespace: seederv1alpha1.DefaultLocalClusterNamespace,
		},
		Spec: seederv1alpha1.ClusterSpec{
			Nodes:            []seederv1alpha1.NodeConfig{},
			HarvesterVersion: "local",
			VIPConfig:        seederv1alpha1.VIPConfig{},
			ClusterConfig:    seederv1alpha1.ClusterConfig{},
		},
	}

	// check if local cluster already exists before creating the same
	var notFound bool
	existingCluster := &seederv1alpha1.Cluster{}
	err := client.Get(ctx, types.NamespacedName{Namespace: seederv1alpha1.DefaultLocalClusterNamespace, Name: seederv1alpha1.DefaultLocalClusterName}, existingCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			notFound = true
		} else {
			return fmt.Errorf("error looking up local cluster: %v", err)
		}
	}

	if notFound {
		err = client.Create(ctx, localCluster)
		return err
	}

	return nil
}

// updateLocal cluster will update the status of the local cluster
func updateLocalCluster(ctx context.Context, client client.Client) error {
	existingCluster := &seederv1alpha1.Cluster{}
	err := client.Get(ctx, types.NamespacedName{Namespace: seederv1alpha1.DefaultLocalClusterNamespace, Name: seederv1alpha1.DefaultLocalClusterName}, existingCluster)
	if err != nil {
		return fmt.Errorf("error fetching local cluster: %v", err)
	}
	existingCluster.Status.ClusterAddress = seederv1alpha1.DefaultLocalClusterAddress
	existingCluster.Status.Status = seederv1alpha1.ClusterRunning

	return client.Status().Update(ctx, existingCluster)
}

// SetupLocalCluster is a wrapper to setup and create a local cluster
func SetupLocalCluster(ctx context.Context, client client.Client) error {
	err := createLocalCluster(ctx, client)
	if err != nil {
		return err
	}

	return updateLocalCluster(ctx, client)
}
