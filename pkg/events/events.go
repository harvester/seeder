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
	var fanHealth, thermalHealth, health []string
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
				msg := fmt.Sprintf("%s reported temp: %vC", v.Name, v.ReadingCelsius)
				thermalHealth = append(thermalHealth, msg)
			}

			for _, fan := range thermals.Fans {
				if fan.Status.Health != "" && fan.Status.Health != "OK" {
					msg := fmt.Sprintf("fan %s is %s", fan.Name, fan.Status.Health)
					health = append(health, msg)
				}
			}

		}
	}

	if len(fanHealth) > 0 {
		health = append(health, fmt.Sprintf("FanHealth: %s", strings.Join(fanHealth, ",")))
	}

	if len(thermalHealth) > 0 {
		health = append(health, fmt.Sprintf("ThermalHealth: %s", strings.Join(thermalHealth, ",")))
	}

	return health, nil
}

func getDrives(chassis []*redfish.Chassis) ([]string, error) {
	var health, driveHealth []string
	for _, c := range chassis {
		if c.Name != "Computer System Chassis" {
			continue
		}
		drives, err := c.Drives()
		if err != nil {
			return nil, fmt.Errorf("error getting drives for chassis %s: %v", c.Name, err)
		}

		for _, d := range drives {
			// skip if no health check is available
			if d.Status.Health != "" && d.Status.Health != "OK" {
				msg := fmt.Sprintf("%s is %s", d.Name, d.Status.Health)
				driveHealth = append(driveHealth, msg)
			}
		}
	}

	if len(driveHealth) > 0 {
		health = append(health, fmt.Sprintf("DriveHealth: %s", strings.Join(driveHealth, ",")))
	}
	return health, nil
}

func getPower(chassis []*redfish.Chassis) ([]string, error) {
	var health, powerSupplyHealth []string
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
				if supply.Status.Health != "" && supply.Status.Health != "OK" {
					msg := fmt.Sprintf("%s is %s", supply.Name, supply.Status.Health)
					powerSupplyHealth = append(powerSupplyHealth, msg)
				}
			}
		}
	}

	if len(powerSupplyHealth) > 0 {
		health = append(health, fmt.Sprintf("PowerSupplyHealth: %s", strings.Join(powerSupplyHealth, ",")))
	}
	return health, nil
}

func getNetworkAdapters(chassis []*redfish.Chassis) ([]string, error) {
	var health, nicHealth []string
	for _, c := range chassis {
		if c.Name != "Computer System Chassis" {
			continue
		}
		nics, err := c.NetworkAdapters()
		if err != nil {
			return nil, fmt.Errorf("error querying network adapters for chassis %s: %v", c.Name, err)
		}

		for _, n := range nics {
			if n.Status.Health != "" && n.Status.Health != "OK" {
				msg := fmt.Sprintf("%s is %s", n.Name, n.Status.Health)
				nicHealth = append(nicHealth, msg)
			}
		}
	}

	if len(nicHealth) > 0 {
		health = append(health, fmt.Sprintf("NetworkAdapterHealth: %s", strings.Join(nicHealth, ",")))
	}
	return health, nil
}

func getSystemComponents(chassis []*redfish.Chassis) ([]string, error) {
	var health, storageHealth, pciDeviceHealth, memHealth, cpuHealth []string
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
				if s.Status.Health != "" && s.Status.Health != "OK" {
					msg := fmt.Sprintf("%s is %s", s.Name, s.Status.Health)
					storageHealth = append(storageHealth, msg)
				}
			}

			// query pcidevice information
			pcidevices, err := v.PCIeDevices()
			if err != nil {
				return nil, fmt.Errorf("error querying pcidevices in computesystem %s: %v", v.Name, err)
			}

			for _, p := range pcidevices {
				if p.Status.Health != "" && p.Status.Health != "OK" {
					msg := fmt.Sprintf("%s is %s", p.Name, p.Status.Health)
					pciDeviceHealth = append(pciDeviceHealth, msg)
				}
			}

			// query memory information
			memory, err := v.Memory()
			if err != nil {
				return nil, fmt.Errorf("error querying memory in computesystem %s in chassis %s: %v", v.Name, c.Name, err)
			}

			for _, m := range memory {
				if m.Status.Health != "" && m.Status.Health != "OK" {
					msg := fmt.Sprintf("%s is %s", m.Name, m.Status.Health)
					memHealth = append(memHealth, msg)
				}
			}

			// query processor information
			processors, err := v.Processors()
			if err != nil {
				return nil, fmt.Errorf("error querying processors in computesystem %s: %v", v.Name, err)
			}

			for _, p := range processors {
				if p.Status.Health != "" && p.Status.Health != "OK" {
					msg := fmt.Sprintf("%s is %s", p.Name, p.Status.Health)
					cpuHealth = append(cpuHealth, msg)
				}
			}
		}
	}

	if len(storageHealth) > 0 {
		health = append(health, fmt.Sprintf("StorageHealth: %s", strings.Join(storageHealth, ",")))
	}

	if len(pciDeviceHealth) > 0 {
		health = append(health, fmt.Sprintf("PCIDeviceHealth: %s", strings.Join(pciDeviceHealth, ",")))
	}

	if len(memHealth) > 0 {
		health = append(health, fmt.Sprintf("MemoryHealth: %s", strings.Join(memHealth, ",")))
	}

	if len(cpuHealth) > 0 {
		health = append(health, fmt.Sprintf("CPUHealth: %s", strings.Join(cpuHealth, ",")))
	}

	return health, nil
}
