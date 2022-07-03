package events

import (
	"context"
	"fmt"
	"strings"

	"github.com/stmcginnis/gofish"
)

type EventFetcher struct {
	ctx    context.Context
	client *gofish.APIClient
}

const (
	defaultPort = "443"
)

func NewEventFetcher(ctx context.Context, username, password, endpoint string) (*EventFetcher, error) {
	cfg := gofish.ClientConfig{
		Username: username,
		Password: password,
		Endpoint: endpoint,
		Insecure: true,
	}

	apiClient, err := gofish.ConnectContext(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &EventFetcher{ctx: ctx, client: apiClient}, nil
}

func (ef *EventFetcher) GetConfig() (map[string]string, string, error) {
	chassis, err := ef.client.Service.Chassis()
	if err != nil {
		return nil, "", err
	}

	var totalMemory, totalCoreCount, totalCoreHz int
	var manufacturer, model, serialNumber, health string
	for _, v := range chassis {
		if v.Manufacturer == "" {
			continue
		}
		manufacturer = v.Manufacturer
		model = v.Model
		serialNumber = v.SerialNumber
		health = string(v.Status.Health)
		cs, err := v.ComputerSystems()
		if err != nil {
			return nil, "", err
		}
		for _, c := range cs {
			memory, err := c.Memory()
			if err != nil {
				return nil, "", err
			}

			for _, m := range memory {
				totalMemory += m.CapacityMiB
			}

			processors, err := c.Processors()
			if err != nil {
				return nil, "", err
			}
			for _, p := range processors {
				totalCoreCount += p.TotalCores
				totalCoreHz += int(p.MaxSpeedMHz)
			}
		}
	}

	retMap := make(map[string]string)

	retMap["totalCpuCores"] = fmt.Sprintf("%d", totalCoreCount)
	retMap["totalMemoryMiB"] = fmt.Sprintf("%d", totalMemory)
	retMap["totalCoreHz"] = fmt.Sprintf("%d", totalCoreHz)
	retMap["manufacturer"] = strings.ReplaceAll(strings.ReplaceAll(manufacturer, " ", ""), ".", "")
	retMap["model"] = strings.ReplaceAll(strings.ReplaceAll(model, " ", ""), ".", "")
	retMap["serialNumber"] = strings.ReplaceAll(strings.ReplaceAll(serialNumber, " ", ""), ".", "")
	return retMap, health, nil
}
