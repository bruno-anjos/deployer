package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bruno-anjos/deployer/api"
	genericutils "github.com/bruno-anjos/solution-utils"
	"github.com/bruno-anjos/solution-utils/http_utils"
	log "github.com/sirupsen/logrus"
)

type (
	HierarchyEntry struct {
		DeploymentYAMLBytes []byte
		Parent              *genericutils.Node
		Grandparent         *genericutils.Node
		Child               sync.Map
		Static              bool
		IsOrphan            bool
		NewParentChan       chan<- string
	}

	HierarchyEntryDTO struct {
		Parent      *genericutils.Node
		Grandparent *genericutils.Node
		Child       map[string]*genericutils.Node
		Static      bool
		IsOrphan    bool
	}
)

func (e *HierarchyEntry) GetChildren() map[string]*genericutils.Node {
	entryChildren := map[string]*genericutils.Node{}

	e.Child.Range(func(key, value interface{}) bool {
		childId := key.(typeChildMapKey)
		child := value.(typeChildMapValue)
		entryChildren[childId] = child
		return true
	})

	return entryChildren
}

func (e *HierarchyEntry) ToDTO() *HierarchyEntryDTO {
	return &HierarchyEntryDTO{
		Parent:      e.Parent,
		Grandparent: e.Grandparent,
		Child:       e.GetChildren(),
		Static:      e.Static,
		IsOrphan:    e.IsOrphan,
	}
}

type (
	typeChildMapKey   = string
	typeChildMapValue = *genericutils.Node

	HierarchyTable struct {
		hierarchyEntries sync.Map
	}

	typeHierarchyEntriesMapKey   = string
	typeHierarchyEntriesMapValue = *HierarchyEntry
)

func NewHierarchyTable() *HierarchyTable {
	return &HierarchyTable{
		hierarchyEntries: sync.Map{},
	}
}

func (t *HierarchyTable) AddDeployment(dto *api.DeploymentDTO) bool {
	entry := &HierarchyEntry{
		DeploymentYAMLBytes: dto.DeploymentYAMLBytes,
		Parent:              dto.Parent,
		Grandparent:         dto.Grandparent,
		Child:               sync.Map{},
		Static:              dto.Static,
		IsOrphan:            false,
		NewParentChan:       nil,
	}

	_, loaded := t.hierarchyEntries.LoadOrStore(dto.DeploymentId, entry)
	if loaded {
		return false
	}

	return true
}

func (t *HierarchyTable) RemoveDeployment(deploymentId string) {
	t.hierarchyEntries.Delete(deploymentId)
}

func (t *HierarchyTable) HasDeployment(deploymentId string) bool {
	_, ok := t.hierarchyEntries.Load(deploymentId)
	return ok
}

func (t *HierarchyTable) SetDeploymentParent(deploymentId string, parent *genericutils.Node) {
	value, ok := t.hierarchyEntries.Load(deploymentId)
	if !ok {
		return
	}

	entry := value.(typeHierarchyEntriesMapValue)
	entry.Parent = parent

	if entry.NewParentChan != nil {
		entry.NewParentChan <- parent.Id
		close(entry.NewParentChan)
		entry.NewParentChan = nil
	}

	entry.IsOrphan = false
}

func (t *HierarchyTable) SetDeploymentAsOrphan(deploymentId string) <-chan string {
	value, ok := t.hierarchyEntries.Load(deploymentId)
	if !ok {
		return nil
	}

	entry := value.(typeHierarchyEntriesMapValue)
	entry.IsOrphan = true
	newParentChan := make(chan string)
	entry.NewParentChan = newParentChan

	return newParentChan
}

func (t *HierarchyTable) AddChild(deploymentId string, child *genericutils.Node) bool {
	value, ok := t.hierarchyEntries.Load(deploymentId)
	if !ok {
		return false
	}

	entry := value.(typeHierarchyEntriesMapValue)
	entry.Child.Store(child.Id, child)

	return true
}

func (t *HierarchyTable) RemoveChild(deploymentId, childId string) bool {
	value, ok := t.hierarchyEntries.Load(deploymentId)
	if !ok {
		return false
	}

	entry := value.(typeHierarchyEntriesMapValue)
	entry.Child.Delete(childId)

	return true
}

func (t *HierarchyTable) GetChildren(deploymentId string) (children map[string]*genericutils.Node) {
	value, ok := t.hierarchyEntries.Load(deploymentId)
	if !ok {
		return nil
	}

	entry := value.(typeHierarchyEntriesMapValue)
	return entry.GetChildren()
}

func (t *HierarchyTable) GetParent(deploymentId string) *genericutils.Node {
	value, ok := t.hierarchyEntries.Load(deploymentId)
	if !ok {
		return nil
	}

	entry := value.(typeHierarchyEntriesMapValue)

	return entry.Parent
}

func (t *HierarchyTable) DeploymentToDTO(deploymentId string) (*api.DeploymentDTO, bool) {
	value, ok := t.hierarchyEntries.Load(deploymentId)
	if !ok {
		return nil, false
	}

	entry := value.(typeHierarchyEntriesMapValue)

	return &api.DeploymentDTO{
		Parent:              entry.Parent,
		Grandparent:         entry.Grandparent,
		DeploymentId:        deploymentId,
		Static:              entry.Static,
		DeploymentYAMLBytes: entry.DeploymentYAMLBytes,
	}, true
}

func (t *HierarchyTable) IsStatic(deploymentId string) (bool, bool) {
	value, ok := t.hierarchyEntries.Load(deploymentId)
	if !ok {
		return false, false
	}

	entry := value.(typeHierarchyEntriesMapValue)
	return entry.Static, true
}

func (t *HierarchyTable) RemoveParent(deploymentId string) bool {
	value, ok := t.hierarchyEntries.Load(deploymentId)
	if !ok {
		return false
	}

	entry := value.(typeHierarchyEntriesMapValue)
	entry.Parent = nil

	return true
}

func (t *HierarchyTable) GetGrandparent(deploymentId string) *genericutils.Node {
	value, ok := t.hierarchyEntries.Load(deploymentId)
	if !ok {
		return nil
	}

	entry := value.(typeHierarchyEntriesMapValue)

	return entry.Grandparent
}

func (t *HierarchyTable) RemoveGrandparent(deploymentId string) {
	value, ok := t.hierarchyEntries.Load(deploymentId)
	if !ok {
		return
	}

	entry := value.(typeHierarchyEntriesMapValue)
	entry.Grandparent = nil
}

func (t *HierarchyTable) GetDeployments() []string {
	var deploymentIds []string

	t.hierarchyEntries.Range(func(key, value interface{}) bool {
		deploymentId := key.(typeHierarchyEntriesMapKey)
		deploymentIds = append(deploymentIds, deploymentId)
		return true
	})

	return deploymentIds
}

func (t *HierarchyTable) GetDeploymentConfig(deploymentId string) []byte {
	value, ok := t.hierarchyEntries.Load(deploymentId)
	if !ok {
		return nil
	}

	entry := value.(typeHierarchyEntriesMapValue)
	return entry.DeploymentYAMLBytes
}

func (t *HierarchyTable) GetDeploymentsWithParent(parentId string) (deploymentIds []string) {
	t.hierarchyEntries.Range(func(key, value interface{}) bool {
		deploymentId := key.(typeHierarchyEntriesMapKey)
		deployment := value.(typeHierarchyEntriesMapValue)

		if deployment.Parent.Id == parentId {
			deploymentIds = append(deploymentIds, deploymentId)
		}

		return true
	})

	return
}

func (t *HierarchyTable) ToDTO() map[string]*HierarchyEntryDTO {
	entries := map[string]*HierarchyEntryDTO{}

	t.hierarchyEntries.Range(func(key, value interface{}) bool {
		deploymentId := key.(typeHierarchyEntriesMapKey)
		entry := value.(typeHierarchyEntriesMapValue)
		entries[deploymentId] = entry.ToDTO()
		return true
	})

	return entries
}

const (
	alive     = 1
	suspected = 0
)

type (
	ParentsEntry struct {
		Parent           *genericutils.Node
		NumOfDeployments int32
		IsUp             int32
	}

	ParentsTable struct {
		parentEntries sync.Map
	}

	typeParentEntriesMapValue = *ParentsEntry
)

func NewParentsTable() *ParentsTable {
	return &ParentsTable{
		parentEntries: sync.Map{},
	}
}

func (t *ParentsTable) AddParent(parent *genericutils.Node) {
	parentEntry := &ParentsEntry{
		Parent:           parent,
		NumOfDeployments: 1,
		IsUp:             alive,
	}

	t.parentEntries.Store(parent.Id, parentEntry)
}

func (t *ParentsTable) HasParent(parentId string) bool {
	_, ok := t.parentEntries.Load(parentId)
	return ok
}

func (t *ParentsTable) DecreaseParentCount(parentId string) (isZero bool) {
	isZero = false
	value, ok := t.parentEntries.Load(parentId)
	if !ok {
		return
	}

	parentEntry := value.(typeParentEntriesMapValue)
	atomic.AddInt32(&parentEntry.NumOfDeployments, -1)
	return
}

func (t *ParentsTable) RemoveParent(parentId string) {
	t.parentEntries.Delete(parentId)
}

func (t *ParentsTable) SetParentUp(parentId string) {
	value, ok := t.parentEntries.Load(parentId)
	if !ok {
		return
	}

	parentEntry := value.(typeParentEntriesMapValue)
	atomic.StoreInt32(&parentEntry.IsUp, alive)
}

func (t *ParentsTable) CheckDeadParents() (deadParents []*genericutils.Node) {
	t.parentEntries.Range(func(key, value interface{}) bool {
		parentEntry := value.(typeParentEntriesMapValue)

		isAlive := atomic.CompareAndSwapInt32(&parentEntry.IsUp, alive, suspected)
		if !isAlive {
			deadParents = append(deadParents, parentEntry.Parent)
		}

		return true
	})

	return
}


/*
	HANDLERS
*/

func deadChildHandler(_ http.ResponseWriter, r *http.Request) {
	deploymentId := http_utils.ExtractPathVar(r, DeploymentIdPathVar)
	deadChildId := http_utils.ExtractPathVar(r, DeployerIdPathVar)

	_, ok := suspectedChild.Load(deadChildId)
	if ok {
		log.Debugf("%s deployment from %s reported as dead, but ignored, already negotiating", deploymentId,
			deadChildId)
		return
	}

	grandchild := &genericutils.Node{}
	err := json.NewDecoder(r.Body).Decode(grandchild)
	if err != nil {
		panic(err)
	}

	log.Debugf("grandchild %s reported deployment %s from %s as dead", grandchild.Id, deploymentId, deadChildId)
	suspectedChild.Store(deadChildId, nil)
	hierarchyTable.RemoveChild(deploymentId, deadChildId)
	children.Delete(deadChildId)

	go attemptToExtend(deploymentId, grandchild, "", 0)
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

	hierarchyTable.AddChild(deploymentId, child)
	children.Store(child.Id, child)
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

/*
	HELPER METHODS
*/

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
	waitingTimer := time.NewTimer(waitForNewParentTimeout * time.Second)

	log.Debugf("waiting new parent for %s", deploymentId)

	select {
	case <-waitingTimer.C:
		panic("TODO fallback")
	case newParentId := <-newParentChan:
		log.Debugf("got new parent %s for deployment %s", newParentId, deploymentId)
		return
	}
}

func attemptToExtend(deploymentId string, grandchild *genericutils.Node, location string, maxHops int) {
	extendTimer := time.NewTimer(extendAttemptTimeout * time.Second)

	toExclude := map[string]struct{}{}
	if grandchild != nil {
		toExclude[grandchild.Id] = struct{}{}
	}
	suspectedChild.Range(func(key, value interface{}) bool {
		suspectedId := key.(typeSuspectedChildMapKey)
		toExclude[suspectedId] = struct{}{}
		return true
	})

	var (
		success      bool
		newChildAddr string
		tries        = 0
	)
	for !success {
		newChildAddr = getNodeCloserTo(location, maxHops, toExclude)
		success = extendDeployment(deploymentId, newChildAddr, grandchild)
		tries++
		if tries == 10 {
			log.Errorf("failed to extend deployment %s", deploymentId)
			return
		}
		<-extendTimer.C
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
		log.Debugf("deployment %s is already present in %s", deploymentId, childId)
	} else if status != http.StatusOK {
		log.Errorf("got %d while extending deployment %s to %s", status, deploymentId, childAddr)
		return false
	}

	if grandChild != nil {
		log.Debugf("telling %s to take grandchild %s for deployment %s", childId, grandChild.Id, deploymentId)
		req = http_utils.BuildRequest(http.MethodPost, deployerHostPort, api.GetTakeChildPath(deploymentId),
			grandChild)
		status, _ = http_utils.DoRequest(httpClient, req, nil)
		if status != http.StatusOK {
			log.Errorf("got status %d while attempting to tell %s to take %s as child", status, childId,
				grandChild.Id)
			return false
		}
	}

	log.Debugf("extended %s to %s sucessfully", deploymentId, childId)
	hierarchyTable.AddChild(deploymentId, child)
	children.Store(childId, child)

	return true
}

func loadFallbackHostname(filename string) string {
	fileBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}

	return string(fileBytes)
}
