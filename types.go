package main

import "github.com/docker/go-connections/nat"

type DeploymentYAML struct {
	Spec struct {
		Replicas    int
		ServiceName string
		Template    struct {
			Spec struct {
				Containers []struct {
					Image string
					Env   []struct {
						Name  string
						Value string
					}
					Ports []struct {
						ContainerPort string
					}
				}
			}
		}
	}
}

type Deployment struct {
	DeploymentName    string
	NumberOfInstances int
	Image             string
	EnvVars           []string
	Ports             nat.PortSet
	Static            bool
	InstancesIds      []string
}
