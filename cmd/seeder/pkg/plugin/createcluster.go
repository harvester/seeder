package plugin

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	command "github.com/rancher/wrangler-cli"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type CreateCluster struct {
	Version     string   `usage:"version of harvester" short:"v"`
	Inventory   []string `usage:"list of inventory objects in namespace to be used for cluster"`
	AddressPool string   `usage:"addresspool to be used for address allocation for VIP and inventory nodes"`
	StaticVIP   string   `usage:"[optional] static address for harvester cluster vip (optional). If not specified an address from addresspool will be used"`
	ConfigURL   string   `usage:"[optional] location of common harvester config that will be applied to all nodes"`
	ImageURL    string   `usage:"[optional] location where artifacts for pxe booting inventory are present"`
}

var (
	clusterName                 string
	createClusterPreflightError = errors.New("pre-flight errors detected")
)

func NewCreateCluster() *cobra.Command {
	cc := command.Command(&CreateCluster{}, cobra.Command{
		Short: "create cluster",
		Long: `create-cluster will create a new cluster.metal.harvesterhci.io object from the flags provided. 
It acts as a simple wrapper around the yaml based cluster definition, and aims to be a quick start for provisioning clusters.
For more advanced use cases where additional options need to be provided, please use the yaml based cluster definition method`,
		Use:  "create-cluster $CLUSTER_NAME [options]",
		Args: cobra.ExactArgs(1),
	})
	return cc
}

func (c *CreateCluster) Run(cmd *cobra.Command, args []string) error {
	logrus.Debug(args)
	err := c.preflightchecks(cmd, args)
	if err != nil {
		return err
	}
	cmd.Println(genHeaderMessage(fmt.Sprintf("creating new cluster %s", clusterName)))
	err = c.createCluster(cmd)
	if err != nil {
		return err
	}

	cmd.Println(genHeaderMessage(fmt.Sprintf("cluster %s created", clusterName)))
	return nil
}

// Pre-Run will check if flags are set
func (c *CreateCluster) Pre(cmd *cobra.Command, args []string) error {
	// check flags are set
	var err error
	requiredFlags := []string{"address-pool", "inventory", "version"}
	for _, rf := range requiredFlags {
		if flagErr := cmd.MarkFlagRequired(rf); flagErr != nil {
			err = errors.Wrap(err, flagErr.Error())
		}
	}

	return err
}

func (c *CreateCluster) preflightchecks(cmd *cobra.Command, args []string) error {
	type preFlightFuncs func(*cobra.Command) (bool, error)
	cmd.Println(genHeaderMessage("running pre-flight checks for create-cluster"))
	clusterName = args[0]
	checkList := []preFlightFuncs{
		c.inventoryExists,
		c.addressPoolExists,
		c.clusterExists,
	}

	var preFlightFailures bool
	for _, v := range checkList {
		ok, err := v(cmd)
		if err != nil {
			return err
		}
		preFlightFailures = preFlightFailures || ok
	}

	if preFlightFailures {
		cmd.PrintErrln(genFailMessage("one or more pre-flight checks failed"))
		return createClusterPreflightError
	}

	return nil
}

func (c *CreateCluster) inventoryExists(cmd *cobra.Command) (bool, error) {
	var preCheckFailed bool
	for _, i := range c.Inventory {
		invObj := &seederv1alpha1.Inventory{}
		err := mgmtClient.Get(cmd.Context(), types.NamespacedName{Namespace: namespace, Name: i}, invObj)
		if err != nil {
			if apierrors.IsNotFound(err) {
				preCheckFailed = true
				cmd.Println(genFailMessage(fmt.Sprintf("🖥 unable to find inventory %s in namespace %s", i, namespace)))
				continue
			} else {
				return false, err
			}
		}

		if invObj.Status.Cluster.Name != "" {
			preCheckFailed = true
			cmd.Println(genFailMessage(fmt.Sprintf("🖥 already allocated to cluster %s in namespace %s", invObj.Status.Cluster.Name,
				namespace)))
			continue
		}

		if invObj.Status.Status != seederv1alpha1.InventoryReady {
			preCheckFailed = true
			cmd.Println(genFailMessage(fmt.Sprintf("🖥 inventory %s in namespace %s is not ready for allocation", i,
				namespace)))
			continue
		}

		cmd.Println(genPassMessage(fmt.Sprintf("🖥 inventory %s in namespace %s is ready", i,
			namespace)))

	}

	return preCheckFailed, nil
}

func (c *CreateCluster) addressPoolExists(cmd *cobra.Command) (bool, error) {
	addObj := &seederv1alpha1.AddressPool{}
	err := mgmtClient.Get(cmd.Context(), types.NamespacedName{Namespace: namespace, Name: c.AddressPool}, addObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			cmd.Println(genFailMessage(fmt.Sprintf("🖥 unable to find addresspool %s in namespace %s", c.AddressPool, namespace)))
			return true, nil
		} else {
			return false, err
		}
	}

	if addObj.Status.Status != seederv1alpha1.PoolReady {
		cmd.Println(genFailMessage(fmt.Sprintf("🖥 addresspool %s in namespace %s is not ready", c.AddressPool, namespace)))
		return true, nil
	}

	cmd.Println(genPassMessage(fmt.Sprintf("🖥 addresspool %s in namespace %s is ready", c.AddressPool, namespace)))

	return false, nil
}

func (c *CreateCluster) clusterExists(cmd *cobra.Command) (bool, error) {
	clusterObj := &seederv1alpha1.Cluster{}
	err := mgmtClient.Get(cmd.Context(), types.NamespacedName{Namespace: namespace, Name: clusterName}, clusterObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			cmd.Println(genPassMessage(fmt.Sprintf("🖥 no cluster %s exists in namespace %s", clusterName, namespace)))
			return false, nil
		} else {
			return false, err
		}
	}
	cmd.Println(genFailMessage(fmt.Sprintf("🖥 cluster %s already exists in namespace %s", clusterName, namespace)))
	return true, nil
}

func (c *CreateCluster) generateCluster() *seederv1alpha1.Cluster {
	cluster := &seederv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
		},
		Spec: seederv1alpha1.ClusterSpec{
			HarvesterVersion: c.Version,
			ClusterConfig: seederv1alpha1.ClusterConfig{
				ConfigURL: c.ConfigURL,
			},
		},
	}

	if c.ImageURL != "" {
		cluster.Spec.ImageURL = c.ImageURL
	}

	var nodes []seederv1alpha1.NodeConfig
	for _, v := range c.Inventory {
		nodes = append(nodes, seederv1alpha1.NodeConfig{
			InventoryReference: seederv1alpha1.ObjectReference{
				Name:      v,
				Namespace: namespace,
			},
			AddressPoolReference: seederv1alpha1.ObjectReference{
				Name:      c.AddressPool,
				Namespace: namespace,
			},
		})
	}

	vipConfig := seederv1alpha1.VIPConfig{
		AddressPoolReference: seederv1alpha1.ObjectReference{
			Name:      c.AddressPool,
			Namespace: namespace,
		},
	}

	if c.StaticVIP != "" {
		vipConfig.StaticAddress = c.StaticVIP
	}

	cluster.Spec.Nodes = nodes
	cluster.Spec.VIPConfig = vipConfig
	return cluster
}

func (c *CreateCluster) createCluster(cmd *cobra.Command) error {

	cluster := c.generateCluster()
	err := mgmtClient.Create(cmd.Context(), cluster)
	if err != nil {
		return err
	}
	cmd.Println(genPassMessage(fmt.Sprintf("cluster %s submitted successfully", clusterName)))
	return nil
}
