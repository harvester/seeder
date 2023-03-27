package plugin

import (
	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	command "github.com/rancher/wrangler-cli"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type RecreateCluster struct {
	Version string `usage:"[optional] version to use to recreate cluster" short:"v"`
}

var originalCluster *seederv1alpha1.Cluster

func NewRecreateCluster() *cobra.Command {
	rc := command.Command(&RecreateCluster{}, cobra.Command{
		Short: "recreate cluster",
		Long: `recreate-cluster will extract the config for an existing cluster, delete said cluster and re-create this cluster
using the existing settings. If version is provided, the version will be patched before creating new cluster.`,
		Use:  "recreate-cluster $CLUSTER_NAME",
		Args: cobra.ExactArgs(1),
	})

	return rc
}

func (r *RecreateCluster) Run(cmd *cobra.Command, args []string) error {
	cmd.Println(genHeaderMessage("running pre-flight checks for recreate-cluster"))
	exists, err := r.clusterExists(cmd)
	if err != nil {
		return err
	}

	if !exists {
		return err
	}

	cmd.Println(genHeaderMessage("deleting cluster"))
	err = r.deleteCluster(cmd)
	if err != nil {
		return err
	}

	cmd.Println(genHeaderMessage("recreating cluster"))
	return r.recreateCluster(cmd)
}

func (r *RecreateCluster) Pre(cmd *cobra.Command, args []string) error {
	clusterName = args[0]
	return nil
}

// clusterExists returns true,nil if cluster exists
func (r *RecreateCluster) clusterExists(cmd *cobra.Command) (bool, error) {
	originalCluster = &seederv1alpha1.Cluster{}
	err := mgmtClient.Get(cmd.Context(), types.NamespacedName{Name: clusterName, Namespace: namespace}, originalCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		} else {
			return false, err
		}
	}

	return true, nil
}

func (r *RecreateCluster) deleteCluster(cmd *cobra.Command) error {
	clusterObj := &seederv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
		},
	}
	return mgmtClient.Delete(cmd.Context(), clusterObj)
}

func (r *RecreateCluster) recreateCluster(cmd *cobra.Command) error {
	if r.Version != "" {
		originalCluster.Spec.HarvesterVersion = r.Version
	}
	// cleanup original cluster spec
	originalCluster.Generation = 0
	originalCluster.ResourceVersion = ""
	originalCluster.Finalizers = []string{}
	originalCluster.CreationTimestamp = metav1.Time{}
	originalCluster.DeletionTimestamp = nil
	return mgmtClient.Create(cmd.Context(), originalCluster)
}
