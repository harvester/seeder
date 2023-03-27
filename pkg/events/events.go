package events

import (
	"context"
	"fmt"
	"strings"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
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

func (ef *EventFetcher) GetConfig() (map[string]string, []string, error) {
	chassis, err := ef.client.Service.Chassis()
	if err != nil {
		return nil, nil, err
	}

	var manufacturer, model, serialNumber string

	for _, v := range chassis {
		if v.Manufacturer == "" {
			continue
		}
		manufacturer = v.Manufacturer
		model = v.Model
		serialNumber = v.SerialNumber
	}

	retMap := make(map[string]string)

	retMap["manufacturer"] = trimString(manufacturer)
	retMap["model"] = trimString(model)
	retMap["serialNumber"] = trimString(serialNumber)

	health, err := ef.getChassisInfo()
	return retMap, health, err
}

func trimString(input string) string {
	return strings.ReplaceAll(strings.ReplaceAll(input, " ", ""), ".", "")
}
func (ef *EventFetcher) getChassisInfo() ([]string, error) {
	var health []string
	var err error
	chassis, err := ef.client.Service.Chassis()
	if err != nil {
		return nil, err
	}

	type healthChecker func([]*redfish.Chassis) ([]string, error)

	healthCheckList := []healthChecker{getThermals, getDrives, getPower, getNetworkAdapters, getSystemComponents}
	for _, check := range healthCheckList {
		subSystemHealth, err := check(chassis)
		if err != nil {
			return nil, err
		}
		health = append(health, subSystemHealth...)
	}

	return health, nil
}

func getThermals(chassis []*redfish.Chassis) ([]string, error) {
	var health []string
	for _, c := range chassis {
		if c.Name != "Computer System Chassis" {
			continue
		}
		thermals, err := c.Thermal()
		if err != nil {
			return nil, fmt.Errorf("error querying thermals: %v", err)
		}

		if thermals != nil {
			for _, v := range thermals.Temperatures {
				msg := fmt.Sprintf("thermal reading from sensor %s: %vC", v.Name, v.ReadingCelsius)
				health = append(health, msg)
			}

			for _, fan := range thermals.Fans {
				if fan.Status.Health == "" {
					continue
				}
				msg := fmt.Sprintf("fan %s is %s", fan.Name, fan.Status.Health)
				health = append(health, msg)
			}

		}
	}

	return health, nil
}

func getDrives(chassis []*redfish.Chassis) ([]string, error) {
	var health []string
	for _, c := range chassis {
		if c.Name != "Computer System Chassis" {
			continue
		}
		drives, err := c.ComputerSystems()
		if err != nil {
			return nil, fmt.Errorf("error querying drives for chassis %s: %v", c.Name, err)
		}

		for _, d := range drives {
			// skip if no health check is available
			if d.Status.Health == "" {
				continue
			}
			msg := fmt.Sprintf("drive %s with serial number %s is %s", d.Name, d.SerialNumber, d.Status.Health)
			health = append(health, msg)
		}
	}
	return health, nil
}

func getPower(chassis []*redfish.Chassis) ([]string, error) {
	var health []string
	for _, c := range chassis {
		if c.Name != "Computer System Chassis" {
			continue
		}
		power, err := c.Power()
		if err != nil {
			return nil, fmt.Errorf("error querying power for chassis %s: %v", c.Name, err)
		}

		if power != nil {
			for _, supply := range power.PowerSupplies {
				if supply.Status.Health == "" {
					continue // skip if no health is available
				}
				msg := fmt.Sprintf("power supply %s is %s", supply.Name, supply.Status.Health)
				health = append(health, msg)
			}
		}
	}
	return health, nil
}

func getNetworkAdapters(chassis []*redfish.Chassis) ([]string, error) {
	var health []string
	for _, c := range chassis {
		if c.Name != "Computer System Chassis" {
			continue
		}
		nics, err := c.NetworkAdapters()
		if err != nil {
			return nil, fmt.Errorf("error querying network adapters for chassis %s: %v", c.Name, err)
		}

		for _, n := range nics {
			if n.Status.Health == "" {
				continue
			}
			msg := fmt.Sprintf("network adapter %s is %s", n.Name, n.Status.Health)
			health = append(health, msg)

		}
	}
	return health, nil
}

func getSystemComponents(chassis []*redfish.Chassis) ([]string, error) {
	var health []string
	for _, c := range chassis {
		if c.Name != "Computer System Chassis" {
			continue
		}
		cs, err := c.ComputerSystems()
		if err != nil {
			return nil, fmt.Errorf("error querying computer systems in chassis %s: %v", c.Name, err)
		}

		for _, v := range cs {

			// query storage information
			storage, err := v.Storage()
			if err != nil {
				return nil, err
			}

			for _, s := range storage {
				if s.Status.Health == "" {
					continue
				}
				msg := fmt.Sprintf("storage %s in computeSystem %s is %s", s.Name, v.Name, s.Status.Health)
				health = append(health, msg)
			}

			// query pcidevice information
			pcidevices, err := v.PCIeDevices()
			if err != nil {
				return nil, fmt.Errorf("error querying pcidevices in computesystem %s: %v", v.Name, err)
			}

			for _, p := range pcidevices {
				if p.Status.Health == "" {
					continue
				}
				msg := fmt.Sprintf("pcidevice %s in computeSystem %s is %s", p.Name, v.Name, p.Status.Health)
				health = append(health, msg)
			}

			// query memory information
			memory, err := v.Memory()
			if err != nil {
				return nil, fmt.Errorf("error querying memory in computesystem %s in chassis %s: %v", v.Name, c.Name, err)
			}

			for _, m := range memory {
				if m.Status.Health == "" {
					continue
				}
				msg := fmt.Sprintf("memory %s in computeSystem %s is %s", m.Name, v.Name, m.Status.Health)
				health = append(health, msg)
			}

			// query processor information
			processors, err := v.Processors()
			if err != nil {
				return nil, fmt.Errorf("error querying processors in computesystem %s: %v", v.Name, err)
			}

			for _, p := range processors {
				if p.Status.Health == "" {
					continue
				}
				msg := fmt.Sprintf("processor %s in computeSystem %s is %s", p.Name, v.Name, p.Status.Health)
				health = append(health, msg)
			}
		}
	}
	return health, nil
}
