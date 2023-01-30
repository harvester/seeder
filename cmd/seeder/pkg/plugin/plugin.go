package plugin

import (
	"fmt"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	cli "github.com/rancher/wrangler-cli"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func New() *cobra.Command {
	root := cli.Command(&Plugin{}, cobra.Command{
		Short: "seeder plugin to interact with existing seeder installation bases",
		Long: `seeder plugin allows automation of some routine tasks with seeder.
It can be used as a kubectl plugin by placing it in your path, and renaming the binary as kubectl-seeder or a standalone binary
Currently supported sub commands are:
* gen-kubeconfig: will generate an admin kubeconfig for a harvester cluster provisioned via seeder
* create-cluster: will create a new cluster object with some basic options
* recreate-cluster: will delete and re-create the cluster and patch the version if one is supplied`,
		Use: "seeder -h",
	})
	root.AddCommand(
		NewGenKubeConfig(),
		NewCreateCluster(),
		NewRecreateCluster(),
	)
	return root
}

type Plugin struct {
	Debug     bool   `usage:"enable debug logging" short:"d"`
	Namespace string `usage:"namespace" short:"n"`
}

func (p *Plugin) Run(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("please provide a valid sub-command")
}

func (p *Plugin) PersistentPre(cmd *cobra.Command, args []string) error {
	// enable debug log level at global level
	if p.Debug {
		logrus.SetLevel(logrus.DebugLevel)
		logrus.Debug("debug level enabled")
	}

	// setup k8s client for use with child-commands
	err := seederv1alpha1.AddToScheme(scheme)
	if err != nil {
		return fmt.Errorf("error adding seeder schema: %v", err)
	}
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := kubeConfig.ClientConfig()

	mgmtClient, err = client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("error generating runtime client: %v", err)
	}

	namespace = p.Namespace

	// if no namespace is specified identify an override if applicable from kubeconfig loading
	if p.Namespace == "" {
		namespace, _, err = kubeConfig.Namespace()
		if err != nil {
			return fmt.Errorf("error identifying namespace: %v", err)
		}
	}

	return nil
}
