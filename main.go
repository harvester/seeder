/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
//go:generate /bin/bash scripts/generate
//go:generate /bin/bash scripts/generate-manifest

package main

import (
	"os"

	"github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/harvester/seeder/pkg/controllers"
)

var (
	VERSION = "v0.0.0-dev"
)

func main() {
	var s controllers.Server

	app := cli.NewApp()
	app.Name = "harvester-seeder"
	app.Version = VERSION
	app.Usage = "Harvester Seeder, to help provision and watch harvester cluster hardware"
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "leader-election-namespace",
			EnvVars:     []string{"LEADER_ELECTION_NAMESPACE"},
			Value:       "default",
			Destination: &s.LeaderElectionNamespace,
			Usage:       "namespace used for creation of leader election objects",
		},
		&cli.BoolFlag{
			Name:        "embedded-mode",
			EnvVars:     []string{"SEEDER_EMBEDDED_MODE"},
			Value:       false,
			Destination: &s.EmbeddedMode,
			Usage:       "enable seeder embedded mode, this disables multi-cluster provisioning",
		},
		&cli.StringFlag{
			Name:        "metrics-bind-address",
			EnvVars:     []string{"METRICS_BIND_ADDRESS"},
			Value:       "0.0.0.0:8080",
			Destination: &s.MetricsAddress,
			Usage:       "default metrics bind address",
		},
		&cli.StringFlag{
			Name:        "health-probe-bind-address",
			EnvVars:     []string{"HEALTH_PROBE_BIND_ADDRESS"},
			Value:       "0.0.0.0:8081",
			Destination: &s.ProbeAddress,
			Usage:       "default health probe bind address",
		},
		&cli.BoolFlag{
			Name:        "leader-elect",
			EnvVars:     []string{"LEADER_ELECT"},
			Value:       true,
			Destination: &s.EnableLeaderElection,
			Usage:       "enable leader election",
		},
		&cli.BoolFlag{
			Name:        "debug",
			EnvVars:     []string{"DEBUG"},
			Value:       false,
			Destination: &s.Debug,
			Usage:       "enable debug logging",
		},
	}

	app.Action = func(c *cli.Context) error {
		ctx := ctrl.SetupSignalHandler()
		return s.Start(ctx)
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}

}
