package main

import (
	"bufio"
	"encoding/json"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
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
)

var (
	myself *genericutils.Node

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
	deployerId := uuid.New()
	myself = &genericutils.Node{
		Id:   deployerId.String(),
		Addr: "",
	}

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

	hierarchyTable.AddDeployment(&deploymentDTO)
	if deploymentDTO.Parent != nil {
		parent := deploymentDTO.Parent
		ok := parentsTable.HasParent(parent.Id)
		if !ok {
			parentsTable.AddParent(parent)
		}

		grandparent := deploymentDTO.Grandparent
		if grandparent != nil {

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

	var (
		success      bool
		newChildAddr string
	)
	for !success {
		newChildAddr = getNodeCloserTo("", 0)
		success = extendDeployment(deploymentId, newChildAddr, grandchild)
	}

	newChildId := getDeployerIdFromAddr(newChildAddr)
	hierarchyTable.AddChild(deploymentId, &genericutils.Node{
		Id:   newChildId,
		Addr: newChildAddr,
	})
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

	parent.Addr, _, err = net.SplitHostPort(r.Host)
	if err != nil {
		panic(err)
	}

	hierarchyTable.SetDeploymentParent(deploymentId, parent)
}

func getHierarchyTableHandler(w http.ResponseWriter, _ *http.Request) {
	http_utils.SendJSONReplyOK(w, hierarchyTable.ToDTO())
}

func checkParentHeartbeatsPeriodically() {
	ticker := time.NewTicker(30 * time.Second)
	for {
		<-ticker.C
		deadParents := parentsTable.CheckDeadParents()
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

func extendDeployment(deploymentId, nodeAddr string, grandChild *genericutils.Node) bool {
	dto, ok := hierarchyTable.DeploymentToDTO(deploymentId)
	if !ok {
		log.Errorf("hierarchy table does not contain deployment %s", deploymentId)
		return false
	}

	childGrandparent := hierarchyTable.GetParent(deploymentId)
	dto.Grandparent = childGrandparent
	dto.Parent = myself

	deployerHostPort := nodeAddr + ":" + strconv.Itoa(api.Port)

	childId := getDeployerIdFromAddr(nodeAddr)
	child := &genericutils.Node{
		Id:   childId,
		Addr: nodeAddr,
	}

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
		log.Errorf("got %d while extending deployment %s to %s", status, deploymentId, nodeAddr)
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

	if nodeDeployerId == myself.Id {
		return true
	}

	_, ok := myAlternatives.Load(nodeDeployerId)
	if ok {
		return true
	}

	log.Debugf("added node %s", nodeDeployerId)

	neighbor := &genericutils.Node{
		Id:   nodeDeployerId,
		Addr: addr,
	}

	myAlternatives.Store(nodeDeployerId, neighbor)
	return true
}

func simulateAlternatives() {
	const (
		alternativesFilename = "alternatives.txt"
	)

	go writeMyselfToAlternatives(alternativesFilename)
	go loadAlternativesPeriodically(alternativesFilename)
}

func writeMyselfToAlternatives(alternativesFilename string) {
	ticker := time.NewTicker(30 * time.Second)
	for {
		f, err := os.OpenFile(alternativesFilename, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			log.Error(err)
			<-ticker.C
			continue
		}

		bytes, err := ioutil.ReadFile(alternativesFilename)
		if err != nil {
			log.Error(err)
			<-ticker.C
			continue
		}

		hostname, err := os.Hostname()
		if err != nil {
			log.Error(err)
			<-ticker.C
			continue
		}

		alternatives := string(bytes)
		if strings.Contains(alternatives, hostname) {
			<-ticker.C
			continue
		}

		if _, err = f.WriteString(hostname + "\n"); err != nil {
			log.Error(err)
			<-ticker.C
			continue
		}

		err = f.Close()
		if err != nil {
			log.Error(err)
			<-ticker.C
			continue
		}

		<-ticker.C
	}
}

func loadAlternativesPeriodically(alternativesFilename string) {
	ticker := time.NewTicker(30 * time.Second)

	var hostname string
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	for {
		<-ticker.C

		f, err := os.OpenFile(alternativesFilename, os.O_RDONLY, os.ModePerm)
		if err != nil {
			log.Errorf("error opening file %s: %s", alternativesFilename, err)
			continue
		}

		rd := bufio.NewReader(f)
		for {
			var addr string
			addr, err = rd.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				log.Errorf("read file line error: %v", err)
				break
			}

			if addr == hostname {
				continue
			}

			onNodeUp(addr)

		}

		if err != nil {
			continue
		}

		err = f.Close()
		if err != nil {
			log.Error(err)
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
