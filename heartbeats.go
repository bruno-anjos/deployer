package main

import (
	"net/http"
	"os"
	"time"

	"github.com/bruno-anjos/deployer/api"
	"github.com/bruno-anjos/solution-utils/http_utils"
	log "github.com/sirupsen/logrus"
)

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
			parentsTable.RemoveParent(deadParent.Id)
			filename := alternativesDir + deadParent.Addr
			if _, err := os.Stat(filename); os.IsNotExist(err) {
				os.Remove(filename)
			}
			renegotiateParent(deadParent)
		}
	}
}
