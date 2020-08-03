package main

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/bruno-anjos/deployer/api"
	"github.com/bruno-anjos/solution-utils/http_utils"
)

type (
	typeDeploymentsMapValue = *Deployment
)

var (
	deployments sync.Map
)

func init () {
	deployments = sync.Map{}
}

func getDeploymentsHandler(w http.ResponseWriter, r *http.Request)  {
	deployments.Store()
}

func registerDeploymentHandler( w http.ResponseWriter, r *http.Request) {
	deploymentId := http_utils.ExtractPathVar(r, deploymentIdPathVar)

	var deployment api.DeploymentDTO
	err := json.NewDecoder(r.Body).Decode(&deployment)
	if err != nil {
		panic(err)
	}

	deployments.Store(deploymentId, )
}
