package tink

import (
	"fmt"

	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
)

const (
	HegelDefaultPort = "50061"
	// override images can be defined in the tink-images configmap in seeder naemspace, and will override images in the default workflow
	StreamHarvesterImageKey    = "steam-harvester-image"
	ConfigureHarvesterImageKey = "configure-harvester-image"
	RebootHarvesterImageKey    = "reboot-harvester-image"
)

func GenerateTemplate(svc *corev1.Service, cm *corev1.ConfigMap, i *seederv1alpha1.Inventory, c *seederv1alpha1.Cluster) (*tinkv1alpha1.Template, error) {
	template := &tinkv1alpha1.Template{
		ObjectMeta: metav1.ObjectMeta{
			Name:      i.Name,
			Namespace: i.Namespace,
		},
	}

	data, err := generateDataTemplate(svc, cm, i, c)
	if err != nil {
		return nil, fmt.Errorf("error generating template data: %v", err)
	}

	template.Spec.Data = data
	return template, nil
}

func generateDataTemplate(svc *corev1.Service, cm *corev1.ConfigMap, i *seederv1alpha1.Inventory, c *seederv1alpha1.Cluster) (*string, error) {
	//set StreamHarvester action environment variables
	DefaultStreamHarvesterAction.Environment[DestDisk] = i.Spec.PrimaryDisk
	DefaultStreamHarvesterAction.Environment[ImageURL] = fmt.Sprintf("%s/%s/harvester-%s-amd64.raw.gz", c.Spec.ImageURL, c.Spec.HarvesterVersion, c.Spec.HarvesterVersion)

	//set ConfigureHarvester action environment variables
	DefaultConfigureHarvesterAction.Environment[HarvesterDevice] = i.Spec.PrimaryDisk
	DefaultConfigureHarvesterAction.Environment[HarvesterCloudInitURL] = fmt.Sprintf("http://%s:%s/2009-04-04/user-data",
		svc.Status.LoadBalancer.Ingress[0].IP, HegelDefaultPort)

	// override images if specified
	if cm != nil {
		if val, ok := cm.Data[StreamHarvesterImageKey]; ok && val != "" {
			DefaultStreamHarvesterAction.Image = val
		}

		if val, ok := cm.Data[ConfigureHarvesterImageKey]; ok && val != "" {
			DefaultConfigureHarvesterAction.Image = val
		}

		if val, ok := cm.Data[RebootHarvesterImageKey]; ok && val != "" {
			DefaultRebootNodeAction.Image = val
		}

	}

	// Define action sequence
	DefaultInstallationTask.Actions = []Action{
		DefaultStreamHarvesterAction,
		DefaultConfigureHarvesterAction,
		DefaultRebootNodeAction,
	}

	// Define workflow
	DefaultHarvesterInstallationWorkflow.Tasks = []Task{
		DefaultInstallationTask,
	}

	output, err := yaml.Marshal(&DefaultHarvesterInstallationWorkflow)
	if err != nil {
		return nil, err
	}

	data := string(output)
	return &data, nil
}
