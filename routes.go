package main

import (
	"fmt"
	"net/http"

	"github.com/bruno-anjos/deployer/api"
	"github.com/bruno-anjos/solution-utils/http_utils"
)

// Route names
const (
	getDeploymentsName          = "GET_DEPLOYMENTS"
	registerDeploymentName      = "REGISTER_DEPLOYMENT"
	registerServiceInstanceName = "REGISTER_SERVICE_INSTANCE"
	deleteDeploymentName        = "DELETE_DEPLOYMENT"
	whoAreYouName               = "WHO_ARE_YOU"
	addNodeName                 = "ADD_NODE"
	setAlternativesName         = "SET_ALTERNATIVES"
	qualityNotAssuredName       = "QUALITY_NOT_ASSURED"
	deadChildName               = "DEAD_CHILD"
	takeChildName               = "TAKE_CHILD"
	iAmYourParentName           = "I_AM_YOUR_PARENT"
	getHierarchyTableName       = "GET_TABLE"
	parentAliveName             = "PARENT_ALIVE"

	// scheduler
	heartbeatServiceInstanceName         = "HEARTBEAT_SERVICE_INSTANCE"
	registerHeartbeatServiceInstanceName = "REGISTER_HEARTBEAT"
)

// Path variables
const (
	DeploymentIdPathVar = "deploymentId"
	DeployerIdPathVar   = "deployerId"
	InstanceIdPathVar   = "instanceId"
)

var (
	_deploymentIdPathVarFormatted = fmt.Sprintf(http_utils.PathVarFormat, DeploymentIdPathVar)
	_instanceIdPathVarFormatted   = fmt.Sprintf(http_utils.PathVarFormat, InstanceIdPathVar)
	_deployerIdPathVarFormatted   = fmt.Sprintf(http_utils.PathVarFormat, DeployerIdPathVar)

	deploymentsRoute       = api.DeploymentsPath
	deploymentRoute        = fmt.Sprintf(api.DeploymentPath, _deploymentIdPathVarFormatted)
	addNodeRoute           = api.AddNodePath
	whoAreYouRoute         = api.WhoAreYouPath
	setAlternativesRoute   = fmt.Sprintf(api.SetAlternativesPath, _deployerIdPathVarFormatted)
	deploymentQualityRoute = fmt.Sprintf(api.DeploymentQualityPath, _deploymentIdPathVarFormatted)
	deadChildRoute         = fmt.Sprintf(api.DeadChildPath, _deploymentIdPathVarFormatted, _deployerIdPathVarFormatted)
	takeChildRoute         = fmt.Sprintf(api.TakeChildPath, _deploymentIdPathVarFormatted)
	iAmYourParentRoute     = fmt.Sprintf(api.IAmYourParentPath, _deploymentIdPathVarFormatted)
	getHierarchyTableRoute = api.GetHierarchyTablePath

	// scheduler
	deploymentInstanceAliveRoute = fmt.Sprintf(api.DeploymentInstanceAlivePath, _deploymentIdPathVarFormatted,
		_instanceIdPathVarFormatted)
	deploymentInstanceRoute = fmt.Sprintf(api.DeploymentInstancePath, _deploymentIdPathVarFormatted,
		_instanceIdPathVarFormatted)
	parentAliveRoute = fmt.Sprintf(api.ParentAlivePath, _deployerIdPathVarFormatted)
)

var routes = []http_utils.Route{
	{
		Name:        parentAliveName,
		Method:      http.MethodPost,
		Pattern:     parentAliveRoute,
		HandlerFunc: parentAliveHandler,
	},

	{
		Name:        qualityNotAssuredName,
		Method:      http.MethodPost,
		Pattern:     deploymentQualityRoute,
		HandlerFunc: qualityNotAssuredHandler,
	},

	{
		Name:        deadChildName,
		Method:      http.MethodPost,
		Pattern:     deadChildRoute,
		HandlerFunc: deadChildHandler,
	},

	{
		Name:        takeChildName,
		Method:      http.MethodPost,
		Pattern:     takeChildRoute,
		HandlerFunc: takeChildHandler,
	},

	{
		Name:        iAmYourParentName,
		Method:      http.MethodPost,
		Pattern:     iAmYourParentRoute,
		HandlerFunc: iAmYourParentHandler,
	},

	{
		Name:        getHierarchyTableName,
		Method:      http.MethodGet,
		Pattern:     getHierarchyTableRoute,
		HandlerFunc: getHierarchyTableHandler,
	},

	{
		Name:        getDeploymentsName,
		Method:      http.MethodGet,
		Pattern:     deploymentsRoute,
		HandlerFunc: getDeploymentsHandler,
	},

	{
		Name:        registerDeploymentName,
		Method:      http.MethodPost,
		Pattern:     deploymentsRoute,
		HandlerFunc: registerDeploymentHandler,
	},

	{
		Name:        deleteDeploymentName,
		Method:      http.MethodDelete,
		Pattern:     deploymentRoute,
		HandlerFunc: deleteDeploymentHandler,
	},

	{
		Name:        addNodeName,
		Method:      http.MethodPost,
		Pattern:     addNodeRoute,
		HandlerFunc: addNodeHandler,
	},

	{
		Name:        whoAreYouName,
		Method:      http.MethodGet,
		Pattern:     whoAreYouRoute,
		HandlerFunc: whoAreYouHandler,
	},

	{
		Name:        setAlternativesName,
		Method:      http.MethodPost,
		Pattern:     setAlternativesRoute,
		HandlerFunc: setAlternativesHandler,
	},

	{
		Name:        heartbeatServiceInstanceName,
		Method:      http.MethodPut,
		Pattern:     deploymentInstanceAliveRoute,
		HandlerFunc: heartbeatServiceInstanceHandler,
	},

	{
		Name:        registerHeartbeatServiceInstanceName,
		Method:      http.MethodPost,
		Pattern:     deploymentInstanceAliveRoute,
		HandlerFunc: registerHeartbeatServiceInstanceHandler,
	},

	{
		Name:        registerServiceInstanceName,
		Method:      http.MethodPost,
		Pattern:     deploymentInstanceRoute,
		HandlerFunc: registerServiceInstanceHandler,
	},
}
