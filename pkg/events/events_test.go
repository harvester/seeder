package events

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	dockertest "github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"
)

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

	networks, err := pool.NetworksByName("bridge")
	if err != nil {
		log.Fatal(err)
	}

	redfishNodeAddress := redfishMock.GetIPInNetwork(&networks[0])

	time.Sleep(30 * time.Second)
	ef, err = NewEventFetcher(context.TODO(), "root", "calvin", fmt.Sprintf("http://%s:8000", redfishNodeAddress))
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
	assert := require.New(t)
	_, health, err := ef.GetConfig()
	assert.NoErrorf(err, "expected no error during inventory call")
	assert.Len(health, 1, "expected to find Thermal Health")
	ef.Close()

}
