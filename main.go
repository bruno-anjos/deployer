package main

import (
	"github.com/bruno-anjos/deployer/api"
	utils "github.com/bruno-anjos/solution-utils"
)

const (
	serviceName = "DEPLOYER"
)

func main() {
	utils.StartServer(serviceName, api.DefaultHostPort, api.Port, api.PrefixPath, routes)
}
