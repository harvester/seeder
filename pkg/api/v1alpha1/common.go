package v1alpha1

// common constants not associated with a particular type
const (
	DefaultNS                      = "metal-system"
	TinkConfig                     = "tinkerbell"
	DefaultAPIPort                 = "9345"
	OverrideAPIPortLabel           = "clusterPort.harvesterhci.io"
	OverrideRedfishPortLabel       = "redfishPort.harvesterhci.io"
	EventLoggerName                = "HarvesterHardwareDiscovery"
	WorkflowLoggerName             = "WorkflowEvent"
	MachineReconcileAnnotationName = "harvesterhci.io/machine-reconcile"
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
	DefaultEndpointPort                  = 9090
	DefaultSeederDeploymentService       = "harvester-seeder-endpoint"
	DefaultHegelDeploymentEndpointLookup = "smee"
	ClusterOwnerKey                      = "harvesterhci.io/clusterOwner"
	ClusterOwnerDetailsKey               = "harvesterhci.io/clusterOwnerDetails"
	ExtraFieldKey                        = "username"
)
