package api

import (
	"fmt"
	"net/http"
	"os"
	"time"

	genericutils "github.com/bruno-anjos/solution-utils"
	"github.com/bruno-anjos/solution-utils/http_utils"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	HeartbeatCheckerTimeout = 60
)

func SendHeartbeatInstanceToDeployer(deployerHostPort string) {
	serviceId := os.Getenv(genericutils.ServiceEnvVarName)
	instanceId := os.Getenv(genericutils.InstanceEnvVarName)

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	log.Infof("will start sending heartbeats to %s as %s from %s", deployerHostPort, instanceId, serviceId)

	alivePath := GetDeploymentInstanceAlivePath(serviceId, instanceId)
	req := http_utils.BuildRequest(http.MethodPost, deployerHostPort, alivePath, nil)
	status, _ := http_utils.DoRequest(httpClient, req, nil)

	switch status {
	case http.StatusConflict:
		log.Debugf("service %s instance %s already has a heartbeat sender", serviceId, instanceId)
		return
	case http.StatusOK:
	default:
		panic(errors.New(fmt.Sprintf("received unexpected status %d", status)))
	}

	ticker := time.NewTicker((HeartbeatCheckerTimeout / 3) * time.Second)
	req = http_utils.BuildRequest(http.MethodPut, deployerHostPort, alivePath, nil)
	for {
		<-ticker.C
		log.Info("sending heartbeat to deployer")
		status, _ = http_utils.DoRequest(httpClient, req, nil)

		switch status {
		case http.StatusNotFound:
			log.Warnf("heartbeat to deployer retrieved not found")
		case http.StatusOK:
		default:
			panic(errors.New(fmt.Sprintf("received unexpected status %d", status)))
		}
	}
}
