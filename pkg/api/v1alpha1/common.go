package v1alpha1

// common constants not associated with a particular type
const (
	DefaultNS                = "metal-system"
	TinkConfig               = "tinkerbell"
	DefaultAPIPort           = "9345"
	OverrideAPIPortLabel     = "clusterPort.harvesterhci.io"
	OverrideRedfishPortLabel = "redfishPort.harvesterhci.io"
	EventLoggerName          = "HarvesterHardwareDiscovery"
)

var (
	DefaultAPIPrefix = "rke2"
)

const (
	NodePowerActionShutdown              = "shutdown"
	NodePowerActionPowerOn               = "poweron"
	NodePowerActionReboot                = "reboot"
	NodeJobComplete                      = "complete"
	NodeJobFailed                        = "failed"
	DefaultHarvesterProvisioningTemplate = "default-harvester-template"
	DefaultTinkStackService              = "tink-stack"
	SeederConfig                         = "seeder-config"
)
