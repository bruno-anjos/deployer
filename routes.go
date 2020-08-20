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
	wasAddedName                = "WAS_ADDED"
	setAlternativesName         = "SET_ALTERNATIVES"
	qualityNotAssuredName       = "QUALITY_NOT_ASSURED"

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
	wasAddedRoute          = fmt.Sprintf(api.WasAddedPath, _deployerIdPathVarFormatted)
	whoAreYouRoute         = api.WhoAreYouPath
	setAlternativesRoute   = fmt.Sprintf(api.SetAlternativesPath, _deployerIdPathVarFormatted)
	deploymentQualityRoute = fmt.Sprintf(api.DeploymentQualityPath, _deploymentIdPathVarFormatted)

	// scheduler
	deploymentInstanceAliveRoute = fmt.Sprintf(api.DeploymentInstanceAlivePath, _deploymentIdPathVarFormatted,
		_instanceIdPathVarFormatted)
	deploymentInstanceRoute = fmt.Sprintf(api.DeploymentInstancePath, _deploymentIdPathVarFormatted,
		_instanceIdPathVarFormatted)
)

var routes = []http_utils.Route{
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
		Name:        wasAddedName,
		Method:      http.MethodPost,
		Pattern:     wasAddedRoute,
		HandlerFunc: wasAddedHandler,
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

	{
		Name:        qualityNotAssuredName,
		Method:      http.MethodPost,
		Pattern:     deploymentQualityRoute,
		HandlerFunc: qualityNotAssuredHandler,
	},
}
