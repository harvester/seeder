package events

import (
	"context"
	"strings"

	"github.com/stmcginnis/gofish"
)

type EventFetcher struct {
	ctx    context.Context
	client *gofish.APIClient
}

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

	var manufacturer, model, serialNumber, health string
	for _, v := range chassis {
		if v.Manufacturer == "" {
			continue
		}
		manufacturer = v.Manufacturer
		model = v.Model
		serialNumber = v.SerialNumber
		health = string(v.Status.Health)

	}

	retMap := make(map[string]string)

	retMap["manufacturer"] = strings.ReplaceAll(strings.ReplaceAll(manufacturer, " ", ""), ".", "")
	retMap["model"] = strings.ReplaceAll(strings.ReplaceAll(model, " ", ""), ".", "")
	retMap["serialNumber"] = strings.ReplaceAll(strings.ReplaceAll(serialNumber, " ", ""), ".", "")

	return retMap, health, nil
}
