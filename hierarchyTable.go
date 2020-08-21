package main

import (
	"sync"

	"github.com/bruno-anjos/deployer/api"
	genericutils "github.com/bruno-anjos/solution-utils"
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

	log.Debugf("setting %s as orphan", deploymentId)
	return make(chan string)
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
