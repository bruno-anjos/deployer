package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/bruno-anjos/deployer/api"
	genericutils "github.com/bruno-anjos/solution-utils"
	"github.com/bruno-anjos/solution-utils/http_utils"
	log "github.com/sirupsen/logrus"
)

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
