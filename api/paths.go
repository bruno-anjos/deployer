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

	AddNodePath  = "/node"
	WasAddedPath = "/added/%s"

	WhoAreYouPath = "/who"

	SetAlternativesPath = "/alternatives/%s"

	DeploymentQualityPath = "/deployments/%s/quality"
	DeadChildPath         = "/deployments/%s/deadchild/%s"
	TakeChildPath         = "/deployments/%s/child"
	IAmYourParentPath     = "/deployments/%s/parent"
	GetHierarchyTablePath = "/table"
	ParentAlivePath       = "/parent/%s/up"

	// scheduler
	DeploymentInstanceAlivePath = "/deployments/%s/%s/alive"
	DeploymentInstancePath      = "/deployments/%s/%s"
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

func GetTakeChildPath(deploymentId string) string {
	return PrefixPath + fmt.Sprintf(TakeChildPath, deploymentId)
}

func GetImYourParentPath(deploymentId string) string {
	return PrefixPath + fmt.Sprintf(IAmYourParentPath, deploymentId)
}

func GetAddNodePath() string {
	return PrefixPath + AddNodePath
}

func GetWasAddedPath(myself string) string {
	return PrefixPath + fmt.Sprintf(WasAddedPath, myself)
}

func GetParentAlivePath(parentId string) string {
	return PrefixPath + fmt.Sprintf(ParentAlivePath, parentId)
}

func GetDeploymentQualityPath(deploymentId string) string {
	return PrefixPath + fmt.Sprintf(DeploymentQualityPath, deploymentId)
}

func GetDeadChildPath(deploymentId, deadChildId string) string {
	return PrefixPath + fmt.Sprintf(DeadChildPath, deploymentId, deadChildId)
}

func GetDeploymentInstancePath(deploymentId, instanceId string) string {
	return PrefixPath + fmt.Sprintf(DeploymentInstancePath, deploymentId, instanceId)
}

func GetSetAlternativesPath(nodeId string) string {
	return PrefixPath + fmt.Sprintf(SetAlternativesPath, nodeId)
}

func GetWhoAreYouPath() string {
	return PrefixPath + WhoAreYouPath
}

func GetDeploymentInstanceAlivePath(deploymentId, instanceId string) string {
	return PrefixPath + fmt.Sprintf(DeploymentInstanceAlivePath, deploymentId, instanceId)
}
