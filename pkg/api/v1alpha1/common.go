package v1alpha1

// common constants not associated with a particular type
const (
	DefaultNS                = "metal-system"
	TinkConfig               = "tinkerbell"
	DefaultAPIPort           = "9345"
	OverrideAPIPortLabel     = "clusterPort.harvesterhci.io"
	OverrideRedfishPortLabel = "redfishPort.harvesterhci.io"
)

var (
	DefaultAPIPrefix = "rke2"
)
