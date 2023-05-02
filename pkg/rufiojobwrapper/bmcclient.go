package rufiojobwrapper

import (
	"context"
	"fmt"

	bmclib "github.com/bmc-toolbox/bmclib/v2"
	rufiocontrollers "github.com/tinkerbell/rufio/controllers"
)

func CustomClientFactory(ctx context.Context) rufiocontrollers.BMCClientFactoryFunc {
	// Initializes a bmclib client based on input host and credentials
	// Establishes a connection with the bmc with client.Open
	// Returns a BMCClient
	return func(ctx context.Context, hostIP, port, username, password string) (rufiocontrollers.BMCClient, error) {
		client := bmclib.NewClient(hostIP, port, username, password)

		for i, v := range client.Registry.Drivers {
			if v.Name == "IntelAMT" {
				client.Registry.Drivers = append(client.Registry.Drivers[:i], client.Registry.Drivers[i+1:]...)
			}
		}
		if err := client.Open(ctx); err != nil {
			return nil, fmt.Errorf("failed to open connection to BMC: %v", err)
		}

		return client, nil
	}
}
