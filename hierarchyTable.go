package main

import (
	"sync"

	"github.com/bruno-anjos/deployer/api"
	genericutils "github.com/bruno-anjos/solution-utils"
)

type (
	HierarchyEntry struct {
		DeploymentYAMLBytes []byte
		Parent              *genericutils.Node
		Grandparent         *genericutils.Node
		Child               *genericutils.Node
		Static              bool
	}

	HierarchyTable struct {
		entries sync.Map
	}

	typeEntriesMapKey   = string
	typeEntriesMapValue = *HierarchyEntry
)

func NewHierarchyTable() *HierarchyTable {
	return &HierarchyTable{
		entries: sync.Map{},
	}
}

func (t *HierarchyTable) AddDeployment(dto *api.DeploymentDTO) bool {
	entry := &HierarchyEntry{
		DeploymentYAMLBytes: dto.DeploymentYAMLBytes,
		Parent:              dto.Parent,
		Grandparent:         dto.Grandparent,
		Child:               nil,
		Static:              dto.Static,
	}

	_, loaded := t.entries.LoadOrStore(dto.DeploymentId, entry)
	if loaded {
		return false
	}

	return true
}

func (t *HierarchyTable) RemoveDeployment(deploymentId string) {
	t.entries.Delete(deploymentId)
}

func (t *HierarchyTable) HasDeployment(deploymentId string) bool {
	_, ok := t.entries.Load(deploymentId)
	return ok
}

func (t *HierarchyTable) SetChild(deploymentId string, child *genericutils.Node) bool {
	value, ok := t.entries.Load(deploymentId)
	if !ok {
		return false
	}

	entry := value.(typeEntriesMapValue)
	entry.Child = child

	return true
}

func (t *HierarchyTable) GetChild(deploymentId string) (*genericutils.Node, bool) {
	value, ok := t.entries.Load(deploymentId)
	if !ok {
		return nil, false
	}

	entry := value.(typeEntriesMapValue)

	return entry.Child, true
}

func (t *HierarchyTable) RemoveChild(deploymentId string) bool {
	value, ok := t.entries.Load(deploymentId)
	if !ok {
		return false
	}

	entry := value.(typeEntriesMapValue)
	entry.Child = nil

	return true
}

func (t *HierarchyTable) GetParent(deploymentId string) (*genericutils.Node, bool) {
	value, ok := t.entries.Load(deploymentId)
	if !ok {
		return nil, false
	}

	entry := value.(typeEntriesMapValue)

	return entry.Parent, true
}

func (t *HierarchyTable) DeploymentToDTO(deploymentId string) (*api.DeploymentDTO, bool) {
	value, ok := t.entries.Load(deploymentId)
	if !ok {
		return nil, false
	}

	entry := value.(typeEntriesMapValue)

	return &api.DeploymentDTO{
		Parent:              entry.Parent,
		Grandparent:         entry.Grandparent,
		DeploymentId:        deploymentId,
		Static:              entry.Static,
		DeploymentYAMLBytes: entry.DeploymentYAMLBytes,
	}, true
}

func (t *HierarchyTable) IsStatic(deploymentId string) (bool, bool) {
	value, ok := t.entries.Load(deploymentId)
	if !ok {
		return false, false
	}

	entry := value.(typeEntriesMapValue)
	return entry.Static, true
}

func (t *HierarchyTable) RemoveParent(deploymentId string) bool {
	value, ok := t.entries.Load(deploymentId)
	if !ok {
		return false
	}

	entry := value.(typeEntriesMapValue)
	entry.Parent = nil

	return true
}

func (t *HierarchyTable) GetGrandparent(deploymentId string) (*genericutils.Node, bool) {
	value, ok := t.entries.Load(deploymentId)
	if !ok {
		return nil, false
	}

	entry := value.(typeEntriesMapValue)

	return entry.Grandparent, true
}

func (t *HierarchyTable) RemoveGrandparent(deploymentId string) {
	value, ok := t.entries.Load(deploymentId)
	if !ok {
		return
	}

	entry := value.(typeEntriesMapValue)
	entry.Grandparent = nil
}

func (t *HierarchyTable) GetDeployments() []string {
	var deploymentIds []string

	t.entries.Range(func(key, value interface{}) bool {
		deploymentId := key.(typeEntriesMapKey)
		deploymentIds = append(deploymentIds, deploymentId)
		return true
	})

	return deploymentIds
}

func (t *HierarchyTable) GetDeploymentConfig(deploymentId string) []byte {
	value, ok := t.entries.Load(deploymentId)
	if !ok {
		return nil
	}

	entry := value.(typeEntriesMapValue)
	return entry.DeploymentYAMLBytes
}
