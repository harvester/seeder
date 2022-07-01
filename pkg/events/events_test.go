package events

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var secret = &corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test",
		Namespace: "default",
	},
	StringData: map[string]string{
		"username": "root",
		"password": "calvin",
	},
}

var ef *EventFetcher

func TestMain(m *testing.M) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("error connecting to dockerd: %v", err)
	}

	buildOpts := &dockertest.BuildOptions{
		ContextDir: "./testdata",
	}
	runOpts := &dockertest.RunOptions{
		Name: "redfishmock",
		Cmd: []string{
			"-D",
			"/mockup",
		},
	}

	redfishMock, err := pool.BuildAndRunWithBuildOptions(buildOpts, runOpts)
	if err != nil {
		log.Fatalf("error creating redfish mock container: %v", err)
	}

	time.Sleep(30 * time.Second)
	ef, err = NewEventFetcher(context.TODO(), "root", "calvin", fmt.Sprintf("http://localhost:%s", redfishMock.GetPort("8000/tcp")))
	if err != nil {
		panic(err)
	}
	code := m.Run()

	// cleanup
	if err = pool.Purge(redfishMock); err != nil {
		log.Fatalf("error purging redfish mock container: %v", err)
	}
	os.Exit(code)

}

func Test_GetInventory(t *testing.T) {
	_, health, err := ef.GetConfig()
	assert.NoErrorf(t, err, "expected no error during inventory call")
	assert.Equal(t, health, "OK", "expected health to be ok")
	ef.client.HTTPClient.CloseIdleConnections()
}
