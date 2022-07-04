package util

import (
	"fmt"

	"github.com/k3s-io/k3s/pkg/clientaccess"
)

// FetchKubeConfig is a helper method to fetch remote harvester clusters kubeconfig file
func FetchKubeConfig(clusterAddress, token string) error {
	info, err := clientaccess.ParseAndValidateToken(clusterAddress, token)
	if err != nil {
		return err
	}

	fmt.Println(info)
	return nil
}
