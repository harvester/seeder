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
	NodeActionRequested     = "metal.harvesterhci.io/actionRequested"
	NodeActionStatus        = "metal.harvesterhci.io/actionStatus"
	NodeLastActionRequest   = "metal.harvesterhci.io/lastActionRequested"
	NodePowerActionJobName  = "metal.harvesterhci.io/rufioJobName"
	NodePowerActionShutdown = "shutdown"
	NodePowerActionPowerOn  = "poweron"
	NodePowerActionReboot   = "reboot"
	NodeJobComplete         = "complete"
	NodeJobFailed           = "failed"
)
