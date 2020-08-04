package api

import (
	"fmt"
	"strconv"

	utils "github.com/bruno-anjos/solution-utils"
)

// Paths
const (
	PrefixPath = "/deployer"

	DeploymentsPath = "/deployments"
	DeploymentPath  = "/deployments/%s"
)

const (
	Port = 50002
)

var (
	DefaultHostPort = utils.LocalhostAddr + ":" + strconv.Itoa(Port)
)

func GetDeploymentsPath() string {
	return PrefixPath + DeploymentsPath
}

func GetDeploymentPath(deploymentId string) string {
	return PrefixPath + fmt.Sprintf(DeploymentPath, deploymentId)
}
