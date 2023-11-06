package endpoint

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

	seederv1alpha1 "github.com/harvester/seeder/pkg/api/v1alpha1"
	"github.com/stretchr/testify/require"
	tinkv1alpha1 "github.com/tinkerbell/tink/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/harvester/seeder/pkg/mock"
)

var (
	hardwareObjects = `
apiVersion: tinkerbell.org/v1alpha1
kind: Hardware
metadata:
  name: hp-151
  namespace: default
spec:
  disks:
  - device: /dev/sda
  interfaces:
  - dhcp:
      arch: x86_64
      hostname: hp-151-seeder
      ip:
        address: 172.19.108.3
        gateway: 172.19.104.1
        netmask: 255.255.248.0
      lease_time: 86400
      mac: 5c:b9:01:88:d3:75
      uefi: true
    netboot:
      allowPXE: true
      osie:
        baseURL: http://172.19.96.23/iso/
  metadata:
    facility:
      facility_code: on_prem
    instance:
      operating_system:
        distro: harvester
        version: v1.2.0
---
apiVersion: tinkerbell.org/v1alpha1
kind: Hardware
metadata:
  name: hp-153
  namespace: default
spec:
  disks:
  - device: /dev/sda
  interfaces:
  - dhcp:
      arch: x86_64
      hostname: hp-153-seeder
      ip:
        address: 172.19.108.4
        gateway: 172.19.104.1
        netmask: 255.255.248.0
      lease_time: 86400
      mac: 5c:b9:01:88:d3:75
      uefi: true
    netboot:
      allowPXE: true
      osie:
        baseURL: http://172.19.96.23/iso/
  metadata:
    facility:
      facility_code: on_prem
    instance:
      operating_system:
        distro: harvester
        version: v1.2.0		
`
	fakeclient client.WithWatch
)

func TestMain(t *testing.M) {
	log := zap.New(zap.UseDevMode(true))
	ctx, cancel := context.WithCancel(context.TODO())
	objs, err := mock.GenerateObjectsFromVar(hardwareObjects)
	if err != nil {
		log.Error(err, "error generating hw objects")
		os.Exit(1)
	}
	fakeclient, err = mock.GenerateFakeClientFromObjects(objs)
	if err != nil {
		log.Error(err, "error generating mock client")
		os.Exit(1)
	}

	s := NewServer(ctx, fakeclient, log)
	go func() {
		if err := s.Start(); err != nil {
			log.Error(err, "error starting server")
			os.Exit(1)
		}
	}()

	code := t.Run()
	cancel()
	os.Exit(code)
}

func Test_disableHardware(t *testing.T) {
	var tests = []struct {
		name             string
		namespace        string
		httpStatusCode   int
		checkPXEDisabled bool
	}{
		{
			name:             "hp-151",
			namespace:        "default",
			httpStatusCode:   202,
			checkPXEDisabled: true,
		},
		{
			name:             "hp-154",
			namespace:        "default",
			httpStatusCode:   404,
			checkPXEDisabled: false,
		},
	}
	assert := require.New(t)
	for _, t := range tests {
		req, err := http.NewRequest("PUT", fmt.Sprintf("http://localhost:%d/disable/%s/%s", seederv1alpha1.DefaultEndpointPort, t.namespace, t.name), nil)
		assert.NoError(err, fmt.Sprintf("expected no error during generation of request for test %s", t.name))
		resp, err := http.DefaultClient.Do(req)
		assert.NoErrorf(err, fmt.Sprintf("error making call for test %s", t.name))
		assert.Equal(resp.StatusCode, t.httpStatusCode)
		if t.checkPXEDisabled {
			hwObj := &tinkv1alpha1.Hardware{}
			err = fakeclient.Get(context.TODO(), types.NamespacedName{Name: t.name, Namespace: t.namespace}, hwObj)
			assert.NoError(err, "expected no error looking up object")
			var disabled bool
			for _, v := range hwObj.Spec.Interfaces {
				disabled = disabled || *v.Netboot.AllowPXE
			}
			assert.False(disabled)
		}
	}

}
