package main

import (
	"bufio"
	"encoding/json"
	"math/rand"
	"net"
	"net/http"
	"os"
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
	typeMyAlternativesMapValue = *genericutils.Node

	typeChildrenMapValue = *genericutils.Node
)

const (
	sendAlternativesTimeout = 30

	maxHopsToLookFor = 5
)

var (
	myself *genericutils.Node

	httpClient *http.Client

	myAlternatives       sync.Map
	nodeAlternatives     map[string][]*genericutils.Node
	nodeAlternativesLock sync.RWMutex
	hierarchyTable       *HierarchyTable

	children sync.Map

	timer *time.Timer
)

func init() {
	deployerId := uuid.New()
	myself = &genericutils.Node{
		Id:   deployerId.String(),
		Addr: "",
	}

	httpClient = &http.Client{
		Timeout: 10 * time.Second,
	}

	myAlternatives = sync.Map{}
	nodeAlternatives = map[string][]*genericutils.Node{}
	nodeAlternativesLock = sync.RWMutex{}
	hierarchyTable = NewHierarchyTable()

	children = sync.Map{}

	timer = time.NewTimer(sendAlternativesTimeout * time.Second)

	go loadAlternatives()
	go sendAlternativesPeriodically()
}

func qualityNotAssuredHandler(w http.ResponseWriter, r *http.Request) {
	deploymentId := http_utils.ExtractPathVar(r, DeployerIdPathVar)

	location := ""
	err := json.NewDecoder(r.Body).Decode(&location)
	if err != nil {
		panic(err)
	}

	nodeAddr := getNodeCloserTo(location, maxHopsToLookFor)

	if nodeAddr == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	extendDeployment(deploymentId, nodeAddr)
}

func getDeploymentsHandler(w http.ResponseWriter, _ *http.Request) {
	deployments := hierarchyTable.GetDeployments()
	http_utils.SendJSONReplyOK(w, deployments)
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

	addrFrom, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		panic(err)
	}

	preprocessMessage(&deploymentDTO, addrFrom)

	deployment := deploymentYAMLToDeployment(&deploymentYAML, deploymentDTO.Static)

	hierarchyTable.AddDeployment(&deploymentDTO)

	go addDeploymentAsync(deployment, deploymentDTO.DeploymentId)
}

func deleteDeploymentHandler(_ http.ResponseWriter, r *http.Request) {
	deploymentId := http_utils.ExtractPathVar(r, DeploymentIdPathVar)
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

func wasAddedHandler(_ http.ResponseWriter, r *http.Request) {
	log.Debugf("handling request in wasAddedHandler")
	deployerIdWhoAddedMe := http_utils.ExtractPathVar(r, DeployerIdPathVar)
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		panic(err)
	}
	addNode(deployerIdWhoAddedMe, host)
}

func setAlternativesHandler(_ http.ResponseWriter, r *http.Request) {
	deployerId := http_utils.ExtractPathVar(r, DeployerIdPathVar)

	alternatives := new([]*genericutils.Node)
	err := json.NewDecoder(r.Body).Decode(&alternatives)
	if err != nil {
		panic(err)
	}

	nodeAlternativesLock.Lock()
	defer nodeAlternativesLock.Unlock()

	nodeAlternatives[deployerId] = *alternatives
}

func extendDeployment(deploymentId, nodeAddr string) (success bool) {
	dto, ok := hierarchyTable.DeploymentToDTO(deploymentId)
	if !ok {
		success = false
		log.Errorf("hierarchy table does not contain deployment %s", deploymentId)
		return
	}

	childGrandparent, ok := hierarchyTable.GetParent(deploymentId)
	if !ok {
		success = false
		log.Errorf("hierarchy table does not contain deployment %s", deploymentId)
		return
	}

	dto.Grandparent = childGrandparent
	dto.Parent = myself

	deployerHostPort := nodeAddr + ":" + strconv.Itoa(api.Port)

	req := http_utils.BuildRequest(http.MethodPost, deployerHostPort, api.GetDeploymentPath(deploymentId), dto)
	status, _ := http_utils.DoRequest(httpClient, req, nil)
	if status != http.StatusOK {
		log.Errorf("got %d while extending deployment %s to %s", status, deploymentId, nodeAddr)
		success = false
		return
	}

	childId := getDeployerIdFromAddr(nodeAddr)
	child := &genericutils.Node{
		Id:   childId,
		Addr: nodeAddr,
	}
	hierarchyTable.SetChild(deploymentId, child)
	children.Store(childId, child)

	success = true

	return
}

// TODO function simulation lower API
func getNodeCloserTo(location string, maxHopsToLookFor int) string {
	var (
		alternatives []*genericutils.Node
	)

	myAlternatives.Range(func(key, value interface{}) bool {
		node := value.(typeMyAlternativesMapValue)
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

func getDeployerIdFromAddr(addr string) string {
	var nodeDeployerId string

	otherDeployerAddr := addr + ":" + strconv.Itoa(api.Port)
	req := http_utils.BuildRequest(http.MethodGet, otherDeployerAddr, api.GetWhoAreYouPath(), nil)
	status, _ := http_utils.DoRequest(httpClient, req, &nodeDeployerId)

	log.Debugf("other deployer id is %s", nodeDeployerId)

	if status != http.StatusOK {
		log.Fatalf("got status code %d from other deployer", status)
	}

	return nodeDeployerId
}

func addNode(nodeDeployerId, addr string) bool {
	if nodeDeployerId == "" {
		nodeDeployerId = getDeployerIdFromAddr(addr)
	}

	neighbor := &genericutils.Node{
		Id:   nodeDeployerId,
		Addr: addr,
	}

	myAlternatives.Store(nodeDeployerId, neighbor)
	return true
}

func loadAlternatives() {
	time.Sleep(120 * time.Second)

	const (
		alternativesFilename = "alternatives.txt"
	)

	f, err := os.OpenFile(alternativesFilename, os.O_RDONLY, os.ModePerm)
	if err != nil {
		log.Fatalf("error opening file %s: %s", alternativesFilename, err)
		return
	}
	defer func() {
		err = f.Close()
		if err != nil {
			panic(err)
		}
	}()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		addr := sc.Text()
		onNodeUp(addr)
	}

	if err = sc.Err(); err != nil {
		log.Fatalf("scan file error: %v", err)
		return
	}
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
	id := getDeployerIdFromAddr(addr)
	myAlternatives.Delete(id)
	sendAlternatives()
	timer.Reset(sendAlternativesTimeout * time.Second)
}

func preprocessMessage(deployment *api.DeploymentDTO, addrFrom string) {
	if deployment.Parent != nil {
		deployment.Parent.Addr = addrFrom
	}
}

func sendAlternativesPeriodically() {
	for {
		<-timer.C
		sendAlternatives()
		timer.Reset(sendAlternativesTimeout * time.Second)
	}
}

func sendAlternatives() {
	var alternatives []*genericutils.Node
	myAlternatives.Range(func(key, value interface{}) bool {
		neighbor := value.(typeMyAlternativesMapValue)
		alternatives = append(alternatives, neighbor)
		return true
	})

	children.Range(func(key, value interface{}) bool {
		neighbor := value.(typeChildrenMapValue)
		sendAlternativesTo(neighbor, alternatives)
		return true
	})
}

func sendAlternativesTo(neighbor *genericutils.Node, alternatives []*genericutils.Node) {
	req := http_utils.BuildRequest(http.MethodPost, neighbor.Addr, api.GetSetAlternativesPath(myself.Id),
		alternatives)

	status, _ := http_utils.DoRequest(httpClient, req, nil)
	if status != http.StatusOK {
		log.Errorf("got status %d while sending alternatives to %s", status, neighbor.Addr)
	}
}
