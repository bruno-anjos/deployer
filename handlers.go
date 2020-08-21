package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
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

	typeChildrenMapValue = *genericutils.Node
)

const (
	sendAlternativesTimeout = 30

	maxHopsToLookFor = 5

	alternativesDir = "/alternatives/"

	checkParentsTimeout = 30
	heartbeatTimeout    = 10
)

var (
	hostname string
	myself   *genericutils.Node

	httpClient *http.Client

	myAlternatives       sync.Map
	nodeAlternatives     map[string][]*genericutils.Node
	nodeAlternativesLock sync.RWMutex

	hierarchyTable *HierarchyTable
	parentsTable   *ParentsTable

	children sync.Map

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
	children = sync.Map{}

	timer = time.NewTimer(sendAlternativesTimeout * time.Second)

	simulateAlternatives()

	go sendHeartbeatsPeriodically()
	go sendAlternativesPeriodically()
	go checkParentHeartbeatsPeriodically()
}

func qualityNotAssuredHandler(w http.ResponseWriter, r *http.Request) {
	deploymentId := http_utils.ExtractPathVar(r, DeploymentIdPathVar)

	log.Debugf("quality not assured for %s", deploymentId)

	location := ""
	err := json.NewDecoder(r.Body).Decode(&location)
	if err != nil {
		panic(err)
	}

	success := false
	for !success {
		nodeAddr := getNodeCloserTo(location, maxHopsToLookFor)

		if nodeAddr == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		success = extendDeployment(deploymentId, nodeAddr, nil)
	}
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

func setAlternativesHandler(_ http.ResponseWriter, r *http.Request) {
	deployerId := http_utils.ExtractPathVar(r, DeployerIdPathVar)

	alternatives := new([]*genericutils.Node)
	err := json.NewDecoder(r.Body).Decode(alternatives)
	if err != nil {
		panic(err)
	}

	nodeAlternativesLock.Lock()
	defer nodeAlternativesLock.Unlock()

	nodeAlternatives[deployerId] = *alternatives
}

func deadChildHandler(_ http.ResponseWriter, r *http.Request) {
	deploymentId := http_utils.ExtractPathVar(r, DeploymentIdPathVar)
	deadChildId := http_utils.ExtractPathVar(r, DeployerIdPathVar)

	grandchild := &genericutils.Node{}
	err := json.NewDecoder(r.Body).Decode(grandchild)
	if err != nil {
		panic(err)
	}

	log.Debugf("grandchild %s reported deployment %s from %s as dead", grandchild.Id, deploymentId, deadChildId)

	hierarchyTable.RemoveChild(deploymentId, deadChildId)
	children.Delete(deadChildId)

	var (
		success      bool
		newChildAddr string
	)
	for !success {
		newChildAddr = getNodeCloserTo("", 0)
		success = extendDeployment(deploymentId, newChildAddr, grandchild)
	}

	newChildId, err := getDeployerIdFromAddr(newChildAddr)
	if err != nil {
		return
	}
	hierarchyTable.AddChild(deploymentId, genericutils.NewNode(newChildId, newChildAddr))
}

func takeChildHandler(w http.ResponseWriter, r *http.Request) {
	deploymentId := http_utils.ExtractPathVar(r, DeploymentIdPathVar)

	child := &genericutils.Node{}
	err := json.NewDecoder(r.Body).Decode(child)
	if err != nil {
		panic(err)
	}

	log.Debugf("told to accept %s as child for deployment %s", child.Id, deploymentId)

	req := http_utils.BuildRequest(http.MethodPost, child.Addr, api.GetImYourParentPath(deploymentId), myself)
	status, _ := http_utils.DoRequest(httpClient, req, nil)
	if status != http.StatusOK {
		log.Errorf("got status %d while telling %s that im his parent", status, child.Id)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func iAmYourParentHandler(_ http.ResponseWriter, r *http.Request) {
	deploymentId := http_utils.ExtractPathVar(r, DeploymentIdPathVar)

	parent := &genericutils.Node{}
	err := json.NewDecoder(r.Body).Decode(parent)
	if err != nil {
		panic(err)
	}

	log.Debugf("told to accept %s as parent for deployment %s", parent.Id, deploymentId)

	hierarchyTable.SetDeploymentParent(deploymentId, parent)
}

func getHierarchyTableHandler(w http.ResponseWriter, _ *http.Request) {
	http_utils.SendJSONReplyOK(w, hierarchyTable.ToDTO())
}

func parentAliveHandler(_ http.ResponseWriter, r *http.Request) {
	parentId := http_utils.ExtractPathVar(r, DeployerIdPathVar)
	parentsTable.SetParentUp(parentId)
}

func sendHeartbeatsPeriodically() {
	ticker := time.NewTicker(heartbeatTimeout * time.Second)

	for {
		children.Range(func(key, value interface{}) bool {
			child := value.(typeChildrenMapValue)
			req := http_utils.BuildRequest(http.MethodPost, child.Addr, api.GetParentAlivePath(myself.Id), nil)
			status, _ := http_utils.DoRequest(httpClient, req, nil)
			if status != http.StatusOK {
				log.Errorf("got status %d while telling %s that i was alive", status, child.Id)
			}

			return true
		})

		<-ticker.C
	}
}

func checkParentHeartbeatsPeriodically() {
	ticker := time.NewTicker(checkParentsTimeout * time.Second)
	for {
		<-ticker.C
		deadParents := parentsTable.CheckDeadParents()
		if len(deadParents) == 0 {
			log.Debugf("all parents alive")
			continue
		}

		for _, deadParent := range deadParents {
			log.Debugf("dead parent: %+v", deadParent)
			renegotiateParent(deadParent)
		}
	}
}

func renegotiateParent(deadParent *genericutils.Node) {
	deploymentIds := hierarchyTable.GetDeploymentsWithParent(deadParent.Id)

	log.Debugf("renegotiating deployments %+v with parent %s", deploymentIds, deadParent.Id)

	for _, deploymentId := range deploymentIds {
		grandparent := hierarchyTable.GetGrandparent(deploymentId)
		if grandparent == nil {
			panic("TODO fallback")
		}

		newParentChan := hierarchyTable.SetDeploymentAsOrphan(deploymentId)

		req := http_utils.BuildRequest(http.MethodPost, grandparent.Addr,
			api.GetDeadChildPath(deploymentId, deadParent.Id), myself)
		status, _ := http_utils.DoRequest(httpClient, req, nil)
		if status != http.StatusOK {
			log.Errorf("got status %d while renegotiating parent %s with %s for deployment %s", status,
				deadParent, grandparent.Id, deploymentId)
			continue
		}

		go waitForNewDeploymentParent(deploymentId, newParentChan)
	}
}

func waitForNewDeploymentParent(deploymentId string, newParentChan <-chan string) {
	waitingTimer := time.NewTimer(30 * time.Second)

	select {
	case <-waitingTimer.C:
		panic("TODO fallback")
	case newParentId := <-newParentChan:
		log.Debugf("got new parent %s for deployment %s", newParentId, deploymentId)
		return
	}
}

func extendDeployment(deploymentId, childAddr string, grandChild *genericutils.Node) bool {
	dto, ok := hierarchyTable.DeploymentToDTO(deploymentId)
	if !ok {
		log.Errorf("hierarchy table does not contain deployment %s", deploymentId)
		return false
	}

	childGrandparent := hierarchyTable.GetParent(deploymentId)
	dto.Grandparent = childGrandparent
	dto.Parent = myself

	deployerHostPort := addPortToAddr(childAddr)

	childId, err := getDeployerIdFromAddr(childAddr)
	if err != nil {
		return false
	}

	if grandChild != nil && grandChild.Id == childId {
		return false
	}

	child := genericutils.NewNode(childId, childAddr)

	log.Debugf("extending deployment %s to %s", deploymentId, childId)

	req := http_utils.BuildRequest(http.MethodPost, deployerHostPort, api.GetDeploymentsPath(), dto)
	status, _ := http_utils.DoRequest(httpClient, req, nil)
	if status == http.StatusConflict {
		if grandChild != nil {
			log.Debugf("child %s already has %s, telling it to take grandchild %s", childId, deploymentId,
				grandChild.Id)
			req = http_utils.BuildRequest(http.MethodPost, deployerHostPort, api.GetTakeChildPath(deploymentId),
				grandChild)
			status, _ = http_utils.DoRequest(httpClient, req, nil)
			if status != http.StatusOK {
				log.Errorf("got status %d while attempting to tell %s to take %s as child", status, childId,
					grandChild.Id)
				return false
			}
		} else {
			log.Debugf("could not extend deployment %s to %s because of conflict", deploymentId, childId)
			return false
		}
	} else if status != http.StatusOK {
		log.Errorf("got %d while extending deployment %s to %s", status, deploymentId, childAddr)
		return false
	}

	log.Debugf("extended %s to %s sucessfully", deploymentId, childId)
	hierarchyTable.AddChild(deploymentId, child)
	children.Store(childId, child)

	return true
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

func getDeployerIdFromAddr(addr string) (string, error) {
	var nodeDeployerId string

	otherDeployerAddr := addPortToAddr(addr)

	req := http_utils.BuildRequest(http.MethodGet, otherDeployerAddr, api.GetWhoAreYouPath(), nil)
	status, _ := http_utils.DoRequest(httpClient, req, &nodeDeployerId)

	log.Debugf("other deployer id is %s", nodeDeployerId)

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

	_, ok := myAlternatives.Load(nodeDeployerId)
	if ok {
		return true
	}

	log.Debugf("added node %s", nodeDeployerId)

	neighbor := genericutils.NewNode(nodeDeployerId, addr)

	myAlternatives.Store(nodeDeployerId, neighbor)
	return true
}

func simulateAlternatives() {
	go writeMyselfToAlternatives()
	go loadAlternativesPeriodically()
}

func writeMyselfToAlternatives() {
	ticker := time.NewTicker(30 * time.Second)
	filename := alternativesDir + addPortToAddr(hostname)

	for {
		if _, err := os.Stat(filename); os.IsNotExist(err) {
			_, err = os.Create(filename)
			if err != nil {
				log.Error(err)
			}
		}

		<-ticker.C
	}
}

func loadAlternativesPeriodically() {
	ticker := time.NewTicker(30 * time.Second)

	for {
		<-ticker.C

		files, err := ioutil.ReadDir(alternativesDir)
		if err != nil {
			log.Error(err)
			continue
		}

		for _, f := range files {
			addr := f.Name()
			if addr == hostname {
				continue
			}

			onNodeUp(addr)
		}
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
	id, err := getDeployerIdFromAddr(addr)
	if err != nil {
		return
	}

	myAlternatives.Delete(id)
	sendAlternatives()
	timer.Reset(sendAlternativesTimeout * time.Second)
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

func addPortToAddr(addr string) string {
	if !strings.Contains(addr, ":") {
		return addr + ":" + strconv.Itoa(api.Port)
	}
	return addr
}
