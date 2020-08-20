package main

import (
	"sync"

	"github.com/docker/go-connections/nat"
)

type DeploymentYAML struct {
	Spec struct {
		Replicas    int
		ServiceName string `yaml:"serviceName"`
		Template    struct {
			Spec struct {
				Containers []struct {
					Image string
					Env   []struct {
						Name  string
						Value string
					}
					Ports []struct {
						ContainerPort string `yaml:"containerPort"`
					}
				}
			}
		}
	}
}

type Deployment struct {
	DeploymentId      string
	NumberOfInstances int
	Image             string
	EnvVars           []string
	Ports             nat.PortSet
	Static            bool
	Lock              *sync.RWMutex
}

type PairServiceIdStatus struct {
	ServiceId string
	IsUp      bool
	Mutex     *sync.Mutex
}
