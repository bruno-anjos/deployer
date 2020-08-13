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
	RegisterPath    = "/deployments/%s/register/%s"

	AddNodePath  = "/node"
	WasAddedPath = "/added/%s"

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

func GetRegisterDeploymentInstancePath(deploymentId, instanceId string) string {
	return PrefixPath + fmt.Sprintf(RegisterPath, deploymentId, instanceId)
}

func GetAddNodePath() string {
	return PrefixPath + AddNodePath
}

func GetWasAddedPath(myself string) string {
	return PrefixPath + fmt.Sprintf(WasAddedPath, myself)
}

func GetWhoAreYouPath() string {
	return PrefixPath + WhoAreYouPath
}
