package main

import (
	"fmt"
	"net/http"

	"github.com/bruno-anjos/deployer/api"
	"github.com/bruno-anjos/solution-utils/http_utils"
)

// Route names
const (
	getDeploymentsName             = "GET_DEPLOYMENTS"
	registerDeploymentName         = "REGISTER_DEPLOYMENT"
	registerDeploymentInstanceName = "REGISTER_DEPLOYMENT_INSTANCE"
	deleteDeploymentName           = "DELETE_DEPLOYMENT"
	whoAreYouName                  = "WHO_ARE_YOU"
	addNodeName                    = "ADD_NODE"
)

// Path variables
const (
	DeploymentIdPathVar = "deploymentId"
	InstanceIdPathVar   = "instanceId"
)

var (
	_deploymentIdPathVarFormatted = fmt.Sprintf(http_utils.PathVarFormat, DeploymentIdPathVar)
	_instanceIdPathVarFormatted   = fmt.Sprintf(http_utils.PathVarFormat, InstanceIdPathVar)

	deploymentsRoute                = api.DeploymentsPath
	deploymentRoute                 = fmt.Sprintf(api.DeploymentPath, _deploymentIdPathVarFormatted)
	registerDeploymentInstanceRoute = fmt.Sprintf(api.RegisterPath, _deploymentIdPathVarFormatted,
		_instanceIdPathVarFormatted)
	addNodeRoute   = api.AddNodePath
	whoAreYouRoute = api.WhoAreYouPath
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
		Name:        registerDeploymentInstanceName,
		Method:      http.MethodPost,
		Pattern:     registerDeploymentInstanceRoute,
		HandlerFunc: registerDeploymentInstanceHandler,
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
}
