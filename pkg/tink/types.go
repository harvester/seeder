package tink

// struct copied from https://github.com/tinkerbell/tink/blob/main/internal/workflow/types.go
// since they are internal packages this makes it harder to reuse these for generating templates
// Workflow represents a workflow to be executed.
type Workflow struct {
	Version       string `yaml:"version"`
	Name          string `yaml:"name"`
	ID            string `yaml:"id"`
	GlobalTimeout int    `yaml:"global_timeout"`
	Tasks         []Task `yaml:"tasks"`
}

// Task represents a task to be executed as part of a workflow.
type Task struct {
	Name        string            `yaml:"name"`
	WorkerAddr  string            `yaml:"worker"`
	Actions     []Action          `yaml:"actions"`
	Volumes     []string          `yaml:"volumes,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
}

// Action is the basic executional unit for a workflow.
type Action struct {
	Name        string            `yaml:"name"`
	Image       string            `yaml:"image"`
	Timeout     int64             `yaml:"timeout"`
	Command     []string          `yaml:"command,omitempty"`
	OnTimeout   []string          `yaml:"on-timeout,omitempty"`
	OnFailure   []string          `yaml:"on-failure,omitempty"`
	Volumes     []string          `yaml:"volumes,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	Pid         string            `yaml:"pid,omitempty"`
}

const (
	HarvesterDevice       = "HARVESTER_DEVICE"
	DestDisk              = "DEST_DISK"
	HarvesterCloudInitURL = "HARVESTER_STREAMDISK_CLOUDINIT_URL"
	ImageURL              = "IMG_URL"
)

var (
	DefaultHarvesterInstallationWorkflow = Workflow{
		Name:          "harvester-installation",
		Version:       "0.1",
		GlobalTimeout: 36000,
	}

	DefaultInstallationTask = Task{
		Name:       "harvester-os-installation",
		WorkerAddr: "{{.device_1}}",
		Volumes:    []string{"/dev:/dev", "/dev/console:/dev/console", "/lib/firmware:/lib/firmware"},
	}
	DefaultStreamHarvesterAction = Action{
		Name:    "stream-harvester",
		Image:   "gmehta3/image2disk:dev",
		Timeout: 3000,
		Environment: map[string]string{
			"COMPRESSED": "true",
			DestDisk:     "", // inventory disk info where image is copied to
			ImageURL:     "", // location where raw.gz image artifact is stored
		},
	}
	DefaultConfigureHarvesterAction = Action{
		Name:    "configure-harvester",
		Image:   "gmehta3/configure-harvester:latest",
		Timeout: 90,
		Environment: map[string]string{
			"HARVESTER_TTY":       "tty1",
			HarvesterDevice:       "", // inventory disk info, where image is installed
			HarvesterCloudInitURL: "", // hegel endopint reference
		},
	}
	DefaultRebootNodeAction = Action{
		Name:    "reboot-harvester",
		Image:   "gmehta3/reboot:latest",
		Timeout: 90,
		Volumes: []string{"/worker:/worker"},
	}
)
