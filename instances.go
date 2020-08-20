package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	archimedes "github.com/bruno-anjos/archimedes/api"
	"github.com/bruno-anjos/deployer/api"
	scheduler "github.com/bruno-anjos/scheduler/api"
	"github.com/bruno-anjos/solution-utils/http_utils"
	log "github.com/sirupsen/logrus"
)

type (
	typeHeartbeatsMapKey   = string
	typeHeartbeatsMapValue = *PairServiceIdStatus

	typeInitChansMapValue = chan struct{}
)

const (
	initInstanceTimeout = 30 * time.Second
)

var (
	heartbeatsMap sync.Map
	initChansMap  sync.Map
)

func init() {
	heartbeatsMap = sync.Map{}
	initChansMap = sync.Map{}

	go instanceHeartbeatChecker()
}

func registerServiceInstanceHandler(w http.ResponseWriter, r *http.Request) {
	log.Debug("handling request in registerServiceInstance handler")

	deploymentId := http_utils.ExtractPathVar(r, DeploymentIdPathVar)

	ok := hierarchyTable.HasDeployment(deploymentId)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	instanceId := http_utils.ExtractPathVar(r, InstanceIdPathVar)

	instanceDTO := scheduler.InstanceDTO{}
	err := json.NewDecoder(r.Body).Decode(&instanceDTO)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if !instanceDTO.Static {
		initChan := make(chan struct{})
		initChansMap.Store(instanceId, initChan)
		go cleanUnresponsiveInstance(deploymentId, instanceId, &instanceDTO, initChan)
	} else {
		req := http_utils.BuildRequest(http.MethodPost, archimedes.DefaultHostPort,
			archimedes.GetServiceInstancePath(deploymentId, instanceId), instanceDTO)
		status, _ := http_utils.DoRequest(httpClient, req, nil)
		if status != http.StatusOK {
			log.Debugf("got status %d while adding instance %s to archimedes", status, instanceId)
			w.WriteHeader(status)
			return
		}
		log.Debugf("warned archimedes that instance %s from service %s exists", instanceId, deploymentId)
	}
}

func registerHeartbeatServiceInstanceHandler(w http.ResponseWriter, r *http.Request) {
	serviceId := http_utils.ExtractPathVar(r, DeploymentIdPathVar)
	instanceId := http_utils.ExtractPathVar(r, InstanceIdPathVar)

	pairServiceStatus := &PairServiceIdStatus{
		ServiceId: serviceId,
		IsUp:      true,
		Mutex:     &sync.Mutex{},
	}

	_, loaded := heartbeatsMap.LoadOrStore(instanceId, pairServiceStatus)
	if loaded {
		w.WriteHeader(http.StatusConflict)
		return
	}

	value, initChanOk := initChansMap.Load(instanceId)
	if !initChanOk {
		log.Warnf("ignoring heartbeat from instance %s since it didnt have an init channel", instanceId)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	initChan := value.(typeInitChansMapValue)
	close(initChan)

	log.Debugf("registered service %s instance %s first heartbeat", serviceId, instanceId)
}

func heartbeatServiceInstanceHandler(w http.ResponseWriter, r *http.Request) {
	log.Debug("handling request in heartbeatService handler")

	deploymentId := http_utils.ExtractPathVar(r, DeploymentIdPathVar)

	hierarchyTable.HasDeployment(deploymentId)

	instanceId := http_utils.ExtractPathVar(r, InstanceIdPathVar)

	value, ok := heartbeatsMap.Load(instanceId)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	pairServiceStatus := value.(typeHeartbeatsMapValue)
	pairServiceStatus.Mutex.Lock()
	pairServiceStatus.IsUp = true
	pairServiceStatus.Mutex.Unlock()

	log.Debugf("got heartbeat from instance %s", instanceId)
}

func cleanUnresponsiveInstance(serviceId, instanceId string, instanceDTO *scheduler.InstanceDTO,
	alive <-chan struct{}) {
	unresponsiveTimer := time.NewTimer(initInstanceTimeout)

	select {
	case <-alive:
		log.Debugf("instance %s is up", instanceId)
		req := http_utils.BuildRequest(http.MethodPost, archimedes.DefaultHostPort,
			archimedes.GetServiceInstancePath(serviceId, instanceId), instanceDTO)
		status, _ := http_utils.DoRequest(httpClient, req, nil)

		if status != http.StatusOK {
			log.Errorf("got status %d while registering service %s instance %s", status, serviceId, instanceId)
		}

		return
	case <-unresponsiveTimer.C:
		removeInstance(serviceId, instanceId)
	}
}

func instanceHeartbeatChecker() {
	heartbeatTimer := time.NewTimer(api.HeartbeatCheckerTimeout * time.Second)

	var toDelete []string
	for {
		toDelete = []string{}
		<-heartbeatTimer.C
		log.Debug("checking heartbeats")
		heartbeatsMap.Range(func(key, value interface{}) bool {
			instanceId := key.(typeHeartbeatsMapKey)
			pairServiceStatus := value.(typeHeartbeatsMapValue)
			pairServiceStatus.Mutex.Lock()

			// case where instance didnt set online status since last status reset, so it has to be removed
			if !pairServiceStatus.IsUp {
				pairServiceStatus.Mutex.Unlock()
				removeInstance(pairServiceStatus.ServiceId, instanceId)

				toDelete = append(toDelete, instanceId)
				log.Debugf("removing instance %s", instanceId)
			} else {
				pairServiceStatus.IsUp = false
				pairServiceStatus.Mutex.Unlock()
			}

			return true
		})

		for _, instanceId := range toDelete {
			log.Debugf("removing %s instance from expected hearbeats map", instanceId)
			heartbeatsMap.Delete(instanceId)
		}
		heartbeatTimer.Reset(api.HeartbeatCheckerTimeout * time.Second)
	}
}

func removeInstance(serviceId, instanceId string) {
	req := http_utils.BuildRequest(http.MethodDelete, scheduler.DefaultHostPort,
		scheduler.GetInstancePath(instanceId), nil)
	status, _ := http_utils.DoRequest(httpClient, req, nil)
	if status != http.StatusOK {
		log.Warnf("while trying to remove instance %s after timeout, scheduler returned status %d",
			instanceId, status)
	}

	req = http_utils.BuildRequest(http.MethodDelete, archimedes.DefaultHostPort,
		archimedes.GetServiceInstancePath(serviceId, instanceId), nil)
	status, _ = http_utils.DoRequest(httpClient, req, nil)
}
