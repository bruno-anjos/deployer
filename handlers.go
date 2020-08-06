package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	genericutils "github.com/bruno-anjos/solution-utils"
	"github.com/docker/go-connections/nat"
	log "github.com/sirupsen/logrus"

	archimedes "github.com/bruno-anjos/archimedes/api"
	"github.com/bruno-anjos/deployer/api"
	scheduler "github.com/bruno-anjos/scheduler/api"
	"github.com/bruno-anjos/solution-utils/http_utils"
	"gopkg.in/yaml.v3"
)

type (
	typeDeploymentsMapValue = *Deployment
)

var (
	httpClient  *http.Client
	deployments sync.Map
)

func init() {
	httpClient = &http.Client{
		Timeout: 10 * time.Second,
	}
	deployments = sync.Map{}
}

func getDeploymentsHandler(w http.ResponseWriter, _ *http.Request) {
	var deploymentsToSend []*Deployment

	deployments.Range(func(key, value interface{}) bool {
		deployment := value.(typeDeploymentsMapValue)
		deploymentsToSend = append(deploymentsToSend, deployment)
		return true
	})

	http_utils.SendJSONReplyOK(w, deploymentsToSend)
}

func registerDeploymentHandler(_ http.ResponseWriter, r *http.Request) {
	log.Debug("handling register deployment request")

	var deploymentDTO api.DeploymentDTO
	err := json.NewDecoder(r.Body).Decode(&deploymentDTO)
	if err != nil {
		panic(err)
	}

	var deploymentYAML DeploymentYAML
	err = yaml.Unmarshal(deploymentDTO.DeploymentYAMLBytes, &deploymentYAML)
	if err != nil {
		panic(err)
	}

	deployment := deploymentYAMLToDeployment(&deploymentYAML, deploymentDTO.Static)
	go addDeploymentAsync(deployment, deploymentDTO.DeploymentName)
}

func deleteDeploymentHandler(w http.ResponseWriter, r *http.Request) {
	deploymentName := http_utils.ExtractPathVar(r, DeploymentIdPathVar)

	value, ok := deployments.Load(deploymentName)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	deployment := value.(typeDeploymentsMapValue)

	go deleteDeploymentAsync(deployment, deploymentName)
}

func addDeploymentAsync(deployment *Deployment, deploymentName string) {
	log.Debugf("adding deployment %s", deploymentName)

	servicePath := archimedes.GetServicePath(deploymentName)

	service := archimedes.ServiceDTO{
		Ports: deployment.Ports,
	}

	req := http_utils.BuildRequest(http.MethodPost, archimedes.DefaultHostPort, servicePath, service)
	status, _ := http_utils.DoRequest(httpClient, req, nil)

	if status != http.StatusOK {
		log.Errorf("got status code %d from archimedes", status)
		return
	}

	containerInstance := scheduler.ContainerInstanceDTO{
		ServiceName: deploymentName,
		ImageName:   deployment.Image,
		Ports:       deployment.Ports,
		Static:      deployment.Static,
		EnvVars:     deployment.EnvVars,
	}

	var instanceIds []string
	var instanceId string
	for i := 0; i < deployment.NumberOfInstances; i++ {
		req = http_utils.BuildRequest(http.MethodPost, scheduler.DefaultHostPort, scheduler.GetInstancesPath(),
			containerInstance)
		var resp *http.Response
		status, resp = http_utils.DoRequest(httpClient, req, nil)

		if status != http.StatusOK {
			log.Errorf("got status code %d from scheduler", status)
			req = http_utils.BuildRequest(http.MethodDelete, archimedes.DefaultHostPort, servicePath, nil)
			status, _ = http_utils.DoRequest(httpClient, req, nil)
			if status != http.StatusOK {
				log.Error("error deleting service that failed initializing")
			}
			return
		}

		err := json.NewDecoder(resp.Body).Decode(&instanceId)
		if err != nil {
			panic(err)
		}

		instanceIds = append(instanceIds, instanceId)
	}

	deployment.InstancesIds = instanceIds
	deployments.Store(deploymentName, deployment)
}

func deleteDeploymentAsync(deployment *Deployment, deploymentName string) {
	servicePath := archimedes.GetServicePath(deploymentName)

	req := http_utils.BuildRequest(http.MethodDelete, archimedes.DefaultHostPort, servicePath, nil)
	status, _ := http_utils.DoRequest(httpClient, req, nil)

	if status != http.StatusOK {
		log.Warnf("got status code %d from archimedes", status)
		return
	}

	for _, instance := range deployment.InstancesIds {
		instancePath := scheduler.GetInstancePath(instance)
		req = http_utils.BuildRequest(http.MethodDelete, scheduler.DefaultHostPort, instancePath, nil)
		status, _ = http_utils.DoRequest(httpClient, req, nil)

		if status != http.StatusOK {
			log.Warnf("got status code %d from scheduler", status)
			return
		}
	}
}

func deploymentYAMLToDeployment(deploymentYAML *DeploymentYAML, static bool) *Deployment {
	log.Debugf("%+v", deploymentYAML)

	numContainers := len(deploymentYAML.Spec.Template.Spec.Containers)
	if numContainers > 1 {
		panic("more than one container per service is not supported")
	} else if numContainers == 0 {
		panic("no container provided")
	}

	containerSpec := deploymentYAML.Spec.Template.Spec.Containers[0]

	envVars := make([]string, len(containerSpec.Env))
	for i, envVar := range containerSpec.Env {
		envVars[i] = envVar.Name + "=" + envVar.Value
	}

	ports := nat.PortSet{}
	for _, port := range containerSpec.Ports {
		natPort, err := nat.NewPort(genericutils.TCP, port.ContainerPort)
		if err != nil {
			panic(err)
		}

		ports[natPort] = struct{}{}
	}

	deployment := Deployment{
		DeploymentName:    deploymentYAML.Spec.ServiceName,
		NumberOfInstances: deploymentYAML.Spec.Replicas,
		Image:             containerSpec.Image,
		EnvVars:           envVars,
		Ports:             ports,
		Static:            static,
	}

	log.Debugf("%+v", deployment)

	return &deployment
}
