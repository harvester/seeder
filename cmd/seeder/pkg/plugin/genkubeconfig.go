package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"

	"github.com/harvester/seeder/pkg/util"

	"k8s.io/apimachinery/pkg/runtime"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"

	command "github.com/rancher/wrangler-cli"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	runningClusters, notRunningClusters, missingClusters []string
	mgmtClient                                           client.Client
	ctx                                                  context.Context
	scheme                                               = runtime.NewScheme()
	namespace                                            string
	runningClusterObjs                                   []seederv1alpha1.Cluster
)

func NewGenKubeConfig() *cobra.Command {
	gkc := command.Command(&GenKubeconfig{}, cobra.Command{
		Short: "generate kubeconfig",
		Long: `gen-kubeconfig will leverage the kubeconfig for a seeder cluster and generate a new kubeconfig for the
target Harvester cluster being provisioned and managed via seeder.
The kubeconfig will be placed in $HOME/.kube unless an alternate path is specified via --path flag.
The name of the generated config file will be same as the name of the cluster.metal.harvesterhci.io object`,
		Use:  "gen-kubeconfig $CLUSTER_NAME",
		Args: cobra.MinimumNArgs(1),
	})
	return gkc
}

type GenKubeconfig struct {
	Path string `usage:"path to place generated harvester cluster kubeconfig" short:"p"`
}

func (g *GenKubeconfig) Run(cmd *cobra.Command, args []string) error {
	ctx = cmd.Context()
	logrus.Debugf("args passsed: %v", args)
	err := g.preflightchecks(cmd, args)
	if err != nil {
		return err
	}
	return g.generateKubeConfig(cmd)
}

// preflightchecks will check if the seeder provisioned cluster is in correct state
// and has appropriate information available before attemping kubeconfig generation
func (g *GenKubeconfig) preflightchecks(cmd *cobra.Command, args []string) error {
	for _, v := range args {
		cluster := &seederv1alpha1.Cluster{}
		err := mgmtClient.Get(ctx, types.NamespacedName{Name: v, Namespace: namespace}, cluster)
		if err != nil {
			if apierrors.IsNotFound(err) {
				missingClusters = append(missingClusters, v)
				continue
			} else {
				return fmt.Errorf("error looking up cluster %s: %v", v, err)
			}
		}

		if cluster.Status.Status == seederv1alpha1.ClusterRunning {
			runningClusters = append(runningClusters, v)
			runningClusterObjs = append(runningClusterObjs, *cluster)
		} else {
			notRunningClusters = append(notRunningClusters, v)
		}
	}

	cmd.Println(genHeaderMessage("running pre-flight checks for gen-kubeconfig"))
	for _, v := range runningClusters {
		cmd.Println(genPassMessage(fmt.Sprintf("cluster %s is running", v)))
	}

	for _, v := range missingClusters {
		cmd.Println(genFailMessage(fmt.Sprintf("cluster %s not found", v)))
	}

	for _, v := range notRunningClusters {
		cmd.Println(genFailMessage(fmt.Sprintf("cluster %s not running", v)))
	}

	return nil

}

func (g *GenKubeconfig) generateKubeConfig(cmd *cobra.Command) error {
	currentLogLevel := logrus.GetLevel()
	defer logrus.SetLevel(currentLogLevel)
	// change log levels temporarily to suppress in build messages from dynamic listener
	// https://github.com/rancher/dynamiclistener/blob/v0.3.3/cert/cert.go#L138
	logrus.SetLevel(logrus.ErrorLevel)

	if len(runningClusterObjs) == 0 {
		cmd.Println(genHeaderMessage("no running clusters specified. no action needed."))
		return nil
	}

	// identify path to write files to
	path := g.Path
	if g.Path == "" {
		home, err := homedir.Dir()
		if err != nil {
			return fmt.Errorf("error evaluating home dir: %v", err)
		}
		path = filepath.Join(home, ".kube")
	}

	cmd.Println(genHeaderMessage("generating kubeconfig for running clusters"))
	for _, v := range runningClusterObjs {
		port, ok := v.Labels[seederv1alpha1.OverrideAPIPortLabel]
		if !ok {
			port = seederv1alpha1.DefaultAPIPort
		}
		kcBytes, err := util.GenerateKubeConfig(v.Status.ClusterAddress, port, seederv1alpha1.DefaultAPIPrefix,
			v.Status.ClusterToken)
		if err != nil {
			return fmt.Errorf("error generating kubeconfig for cluster %s: %v", v.Name, err)
		}

		fileName := filepath.Join(path, fmt.Sprintf("%s-%s.yaml", v.Name, namespace))
		err = os.WriteFile(fileName, kcBytes, 0600)
		if err != nil {
			return fmt.Errorf("error writing kubeconfig for %s: %v", v.Name, err)
		}

		cmd.Println(genPassMessage(fmt.Sprintf("kubeconfig %s-%s.yaml written at %s", v.Name, namespace, path)))
	}
	return nil
}
