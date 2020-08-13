package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	genericutils "github.com/bruno-anjos/solution-utils"
	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	archimedes "github.com/bruno-anjos/archimedes/api"
	"github.com/bruno-anjos/deployer/api"
	scheduler "github.com/bruno-anjos/scheduler/api"
	"github.com/bruno-anjos/solution-utils/http_utils"
	"gopkg.in/yaml.v3"
)

type (
	Deployer struct {
		DeployerId string
		Addr       string
	}

	DeployerCollection struct {
		Deployers map[string]*Deployer
		Mutex     *sync.RWMutex
	}

	typeDeploymentsMapValue = *Deployment

	typeDeployersPerLevelMapValue = *DeployerCollection

	typeDeployersMapKey   = string
	typeDeployersMapValue = *Deployer
)

var (
	deployerId uuid.UUID

	httpClient        *http.Client
	deployments       sync.Map
	deployersPerLevel sync.Map
	deployers         sync.Map
)

func init() {
	deployerId = uuid.New()

	httpClient = &http.Client{
		Timeout: 10 * time.Second,
	}
	deployments = sync.Map{}
	deployersPerLevel = sync.Map{}
	deployers = sync.Map{}
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

func whoAreYouHandler(w http.ResponseWriter, _ *http.Request) {
	http_utils.SendJSONReplyOK(w, deployerId)
}

func addNodeHandler(w http.ResponseWriter, r *http.Request) {
	var nodeAddr string
	err := json.NewDecoder(r.Body).Decode(&nodeAddr)
	if err != nil {
		panic(err)
	}

	if !onNodeUp(nodeAddr, 0) {
		w.WriteHeader(http.StatusConflict)
		return
	}
}

func wasAddedHandler(_ http.ResponseWriter, r *http.Request) {
	log.Debugf("handling request in wasAddedHandler")
	deployerIdWhoAddedMe := http_utils.ExtractPathVar(r, DeployerIdPathVar)
	deployer := Deployer{
		DeployerId: deployerIdWhoAddedMe,
		Addr:       r.RemoteAddr,
	}
	deployers.Store(deployerIdWhoAddedMe, deployer)
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

	for i := 0; i < deployment.NumberOfInstances; i++ {
		req = http_utils.BuildRequest(http.MethodPost, scheduler.DefaultHostPort, scheduler.GetInstancesPath(),
			containerInstance)
		status, _ = http_utils.DoRequest(httpClient, req, nil)

		if status != http.StatusOK {
			log.Errorf("got status code %d from scheduler", status)
			req = http_utils.BuildRequest(http.MethodDelete, archimedes.DefaultHostPort, servicePath, nil)
			status, _ = http_utils.DoRequest(httpClient, req, nil)
			if status != http.StatusOK {
				log.Error("error deleting service that failed initializing")
			}
			return
		}

	}

	deployments.Store(deploymentName, deployment)
}

func registerDeploymentInstanceHandler(w http.ResponseWriter, r *http.Request) {
	deploymentId := http_utils.ExtractPathVar(r, DeploymentIdPathVar)
	instanceId := http_utils.ExtractPathVar(r, InstanceIdPathVar)

	value, ok := deployments.Load(deploymentId)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	deployment := value.(typeDeploymentsMapValue)
	deployment.Lock.Lock()
	deployment.InstancesIds = append(deployment.InstancesIds, instanceId)
	deployment.Lock.Unlock()
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
		InstancesIds:      []string{},
		Lock:              &sync.RWMutex{},
	}

	log.Debugf("%+v", deployment)

	return &deployment
}

func onNodeUp(addr string, level int) bool {
	collection := &DeployerCollection{
		Deployers: map[string]*Deployer{},
		Mutex:     &sync.RWMutex{},
	}
	value, _ := deployersPerLevel.LoadOrStore(level, collection)
	collection = value.(typeDeployersPerLevelMapValue)

	var nodeDeployerId string

	otherDeployerAddr := addr + ":" + strconv.Itoa(api.Port)
	req := http_utils.BuildRequest(http.MethodGet, otherDeployerAddr, api.GetWhoAreYouPath(), nil)
	status, _ := http_utils.DoRequest(httpClient, req, &nodeDeployerId)

	log.Debugf("other deployer id is %s", nodeDeployerId)

	if status != http.StatusOK {
		log.Fatalf("got status code %d from other deployer", status)
	}

	deployer := &Deployer{
		DeployerId: nodeDeployerId,
		Addr:       addr,
	}
	collection.Mutex.Lock()
	collection.Deployers[nodeDeployerId] = deployer
	collection.Mutex.Unlock()

	_, loaded := deployers.LoadOrStore(nodeDeployerId, deployer)
	if loaded {
		return false
	}

	otherArchimedesAddr := addr + ":" + strconv.Itoa(archimedes.Port)

	neighborDTO := archimedes.NeighborDTO{
		Addr: otherArchimedesAddr,
	}

	req = http_utils.BuildRequest(http.MethodPost, archimedes.DefaultHostPort, archimedes.GetNeighborPath(), neighborDTO)
	status, _ = http_utils.DoRequest(httpClient, req, nil)

	if status != http.StatusOK {
		log.Fatalf("got status code %d while adding neighbor in archimedes", status)
	}

	req = http_utils.BuildRequest(http.MethodPost, otherDeployerAddr, api.GetWasAddedPath(deployerId.String()), nil)
	http_utils.DoRequest(httpClient, req, nil)

	if status != http.StatusOK {
		log.Fatalf("got status code %d while alerting %s that i added him", status, otherDeployerAddr)
	}

	return true
}

func onNodeDown(addr string, level int) {

}
