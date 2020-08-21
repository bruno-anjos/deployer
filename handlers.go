package main

import (
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	typeMyAlternativesMapValue = *genericutils.Node
	typeChildrenMapValue       = *genericutils.Node

	typeSuspectedChildMapKey = string
)

// Timeouts
const (
	sendAlternativesTimeout = 30
	checkParentsTimeout     = 30
	heartbeatTimeout        = 10
	extendAttemptTimeout    = 10
	waitForNewParentTimeout = 60
)

const (
	alternativesDir  = "/alternatives/"
	fallbackFilename = "fallback.txt"
	maxHopsToLookFor = 5
)

var (
	hostname string
	fallback string
	location string
	myself   *genericutils.Node

	httpClient *http.Client

	myAlternatives       sync.Map
	nodeAlternatives     map[string][]*genericutils.Node
	nodeAlternativesLock sync.RWMutex

	hierarchyTable *HierarchyTable
	parentsTable   *ParentsTable

	suspectedChild sync.Map
	children       sync.Map

	timer *time.Timer
)

func init() {
	aux, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	hostname = aux + ":" + strconv.Itoa(api.Port)

	deployerId := uuid.New()
	myself = genericutils.NewNode(deployerId.String(), hostname)

	log.Debugf("DEPLOYER_ID: %s", deployerId)

	httpClient = &http.Client{
		Timeout: 10 * time.Second,
	}

	myAlternatives = sync.Map{}
	nodeAlternatives = map[string][]*genericutils.Node{}
	nodeAlternativesLock = sync.RWMutex{}
	hierarchyTable = NewHierarchyTable()
	parentsTable = NewParentsTable()

	suspectedChild = sync.Map{}
	children = sync.Map{}

	timer = time.NewTimer(sendAlternativesTimeout * time.Second)

	// TODO change this for location from lower API
	location = ""
	fallback = loadFallbackHostname(fallbackFilename)

	simulateAlternatives()

	go sendHeartbeatsPeriodically()
	go sendAlternativesPeriodically()
	go checkParentHeartbeatsPeriodically()
}

func qualityNotAssuredHandler(_ http.ResponseWriter, r *http.Request) {
	deploymentId := http_utils.ExtractPathVar(r, DeploymentIdPathVar)

	log.Debugf("quality not assured for %s", deploymentId)

	location := ""
	err := json.NewDecoder(r.Body).Decode(&location)
	if err != nil {
		panic(err)
	}

	go attemptToExtend(deploymentId, nil, location, maxHopsToLookFor)
}

func getDeploymentsHandler(w http.ResponseWriter, _ *http.Request) {
	deployments := hierarchyTable.GetDeployments()
	http_utils.SendJSONReplyOK(w, deployments)
}

func registerDeploymentHandler(w http.ResponseWriter, r *http.Request) {
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

	if hierarchyTable.HasDeployment(deploymentDTO.DeploymentId) {
		w.WriteHeader(http.StatusConflict)
		return
	}

	hierarchyTable.AddDeployment(&deploymentDTO)
	if deploymentDTO.Parent != nil {
		parent := deploymentDTO.Parent
		ok := parentsTable.HasParent(parent.Id)
		if !ok {
			parentsTable.AddParent(parent)
		}
	}

	deployment := deploymentYAMLToDeployment(&deploymentYAML, deploymentDTO.Static)

	go addDeploymentAsync(deployment, deploymentDTO.DeploymentId)
}

func deleteDeploymentHandler(_ http.ResponseWriter, r *http.Request) {
	deploymentId := http_utils.ExtractPathVar(r, DeploymentIdPathVar)

	parent := hierarchyTable.GetParent(deploymentId)
	if parent != nil {
		isZero := parentsTable.DecreaseParentCount(parent.Id)
		if isZero {

		}
	}

	hierarchyTable.RemoveDeployment(deploymentId)

	go deleteDeploymentAsync(deploymentId)
}

func whoAreYouHandler(w http.ResponseWriter, _ *http.Request) {
	http_utils.SendJSONReplyOK(w, myself.Id)
}

func addNodeHandler(_ http.ResponseWriter, r *http.Request) {
	var nodeAddr string
	err := json.NewDecoder(r.Body).Decode(&nodeAddr)
	if err != nil {
		panic(err)
	}

	onNodeUp(nodeAddr)
}

// TODO function simulating lower API
func getNodeCloserTo(location string, maxHopsToLookFor int, excludeNodes map[string]struct{}) string {
	var (
		alternatives []*genericutils.Node
	)

	myAlternatives.Range(func(key, value interface{}) bool {
		node := value.(typeMyAlternativesMapValue)
		if _, ok := excludeNodes[node.Id]; ok {
			return true
		}
		alternatives = append(alternatives, node)
		return true
	})

	if len(alternatives) == 0 {
		return ""
	}

	randIdx := rand.Intn(len(alternatives))
	return alternatives[randIdx].Addr
}

func addDeploymentAsync(deployment *Deployment, deploymentId string) {
	log.Debugf("adding deployment %s", deploymentId)

	servicePath := archimedes.GetServicePath(deploymentId)

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
		ServiceName: deploymentId,
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
			hierarchyTable.RemoveDeployment(deploymentId)
			return
		}
	}
}

func deleteDeploymentAsync(deploymentId string) {
	servicePath := archimedes.GetServicePath(deploymentId)

	req := http_utils.BuildRequest(http.MethodGet, archimedes.DefaultHostPort, servicePath, nil)

	instances := map[string]*archimedes.Instance{}
	status, _ := http_utils.DoRequest(httpClient, req, &instances)
	if status != http.StatusOK {
		log.Errorf("got status %d while requesting service %s instances", status, deploymentId)
		return
	}

	req = http_utils.BuildRequest(http.MethodDelete, archimedes.DefaultHostPort, servicePath, nil)
	status, _ = http_utils.DoRequest(httpClient, req, nil)

	if status != http.StatusOK {
		log.Warnf("got status code %d from archimedes", status)
		return
	}

	for instanceId := range instances {
		instancePath := scheduler.GetInstancePath(instanceId)
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
		DeploymentId:      deploymentYAML.Spec.ServiceName,
		NumberOfInstances: deploymentYAML.Spec.Replicas,
		Image:             containerSpec.Image,
		EnvVars:           envVars,
		Ports:             ports,
		Static:            static,
		Lock:              &sync.RWMutex{},
	}

	log.Debugf("%+v", deployment)

	return &deployment
}

func getDeployerIdFromAddr(addr string) (string, error) {
	var nodeDeployerId string

	otherDeployerAddr := addPortToAddr(addr)

	req := http_utils.BuildRequest(http.MethodGet, otherDeployerAddr, api.GetWhoAreYouPath(), nil)
	status, _ := http_utils.DoRequest(httpClient, req, &nodeDeployerId)

	if status != http.StatusOK {
		log.Errorf("got status code %d from other deployer", status)
		return "", errors.New("got status code %d from other deployer")
	}

	return nodeDeployerId, nil
}

func addNode(nodeDeployerId, addr string) bool {
	if nodeDeployerId == "" {
		var err error
		nodeDeployerId, err = getDeployerIdFromAddr(addr)
		if err != nil {
			return false
		}
	}

	if nodeDeployerId == myself.Id {
		return true
	}

	suspectedChild.Delete(nodeDeployerId)

	_, ok := myAlternatives.Load(nodeDeployerId)
	if ok {
		return true
	}

	log.Debugf("added node %s", nodeDeployerId)

	neighbor := genericutils.NewNode(nodeDeployerId, addr)

	myAlternatives.Store(nodeDeployerId, neighbor)
	return true
}

// TODO function simulation lower API
// Node up is only triggered for nodes that appeared one hop away
func onNodeUp(addr string) {
	addNode("", addr)
	sendAlternatives()
	timer.Reset(sendAlternativesTimeout * time.Second)
}

// TODO function simulation lower API
// Node down is only triggered for nodes that were one hop away
func onNodeDown(addr string) {
	id, err := getDeployerIdFromAddr(addr)
	if err != nil {
		return
	}

	myAlternatives.Delete(id)
	sendAlternatives()
	timer.Reset(sendAlternativesTimeout * time.Second)
}

func addPortToAddr(addr string) string {
	if !strings.Contains(addr, ":") {
		return addr + ":" + strconv.Itoa(api.Port)
	}
	return addr
}
