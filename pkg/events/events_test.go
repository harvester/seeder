package events

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	dockertest "github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/validation"
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

func Test_sanitizeLabelValue(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"hpe synergy backplane", "HPE 2SFF NVMe/SAS/uFF Backplane", "HPE2SFFNVMe-SAS-uFFBackplane"},
		{"already valid is unchanged", "ProLiantDL380Gen10", "ProLiantDL380Gen10"},
		{"spaces stripped as before", "ProLiant DL380 Gen10", "ProLiantDL380Gen10"},
		{"dots stripped as before", "Dell Inc.", "DellInc"},
		{"underscore and hyphen kept", "SYS-1029P_WTR", "SYS-1029P_WTR"},
		{"empty stays empty", "", ""},
		{"comma", "Super Micro Computer, Inc.", "SuperMicroComputer-Inc"},
		{"trailing bracket", "Server (Rev 2)", "Server-Rev2"},
		{"trailing hyphen", "Model X -", "ModelX"},
		{"non-ascii", "Modèle 1", "Mod-le1"},
		{"only invalid characters", "///", ""},
		{"truncated to 63", strings.Repeat("A", 70), strings.Repeat("A", 63)},
		{"truncate then trim", strings.Repeat("A", 62) + "/B", strings.Repeat("A", 62)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeLabelValue(tc.in)
			require.Equal(t, tc.want, got)
			require.Empty(t, validation.IsValidLabelValue(got), "output must be a valid label value")
		})
	}
}
