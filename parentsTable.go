package main

import (
	"sync"
	"sync/atomic"

	generic_utils "github.com/bruno-anjos/solution-utils"
)

const (
	alive = 1
	suspected = 0
)

type (
	ParentsEntry struct {
		Parent           *generic_utils.Node
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

func (t *ParentsTable) AddParent(parent *generic_utils.Node) {
	parentEntry := &ParentsEntry{
		Parent:           parent,
		NumOfDeployments: 1,
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

func (t *ParentsTable) SetParentUp(parentId string) {
	value, ok := t.parentEntries.Load(parentId)
	if !ok {
		return
	}

	parentEntry := value.(typeParentEntriesMapValue)
	atomic.StoreInt32(&parentEntry.IsUp, alive)
}

func (t *ParentsTable) CheckDeadParents() (deadParents []*generic_utils.Node) {
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
