package main

import (
	"fmt"
	"net/http"

	"github.com/bruno-anjos/deployer/api"
	"github.com/bruno-anjos/solution-utils/http_utils"
)

// Route names
const (
	getDeploymentsName     = "GET_DEPLOYMENTS"
	registerDeploymentName = "REGISTER_DEPLOYMENT"
)

// Path variables
const (
	deploymentIdPathVar = "deploymentId"
)

var (
	_deploymentIdPathVarFormatted = fmt.Sprintf(http_utils.PathVarFormat, deploymentIdPathVar)

	deploymentsRoute = api.DeploymentsPath
	deploymentRoute  = fmt.Sprintf(api.DeploymentPath, _deploymentIdPathVarFormatted)
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
}
