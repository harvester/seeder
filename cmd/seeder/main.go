package main

import (
	"github.com/harvester/seeder/cmd/seeder/pkg/plugin"
	command "github.com/rancher/wrangler-cli"
)

func main() {
	command.Main(plugin.New())
}
