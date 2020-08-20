package api

import (
	genericutils "github.com/bruno-anjos/solution-utils"
)

type DeploymentDTO struct {
	Parent              *genericutils.Node
	Grandparent         *genericutils.Node
	DeploymentId        string
	Static              bool
	DeploymentYAMLBytes []byte
}
