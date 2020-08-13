package api

import (
	"fmt"
	"strconv"
)

// Paths
const (
	PrefixPath = "/deployer"

	DeploymentsPath = "/deployments"
	DeploymentPath  = "/deployments/%s"

	AddNodePath = "/node"

	WhoAreYouPath = "/who"
)

const (
	Port = 50002
)

var (
	DeployerServiceName = "deployer"
	DefaultHostPort     = DeployerServiceName + ":" + strconv.Itoa(Port)
)

func GetDeploymentsPath() string {
	return PrefixPath + DeploymentsPath
}

func GetDeploymentPath(deploymentId string) string {
	return PrefixPath + fmt.Sprintf(DeploymentPath, deploymentId)
}

func GetAddNodePath() string {
	return PrefixPath + AddNodePath
}

func GetWhoAreYouPath() string {
	return PrefixPath + WhoAreYouPath
}
