package api

type DeploymentDTO struct {
	DeploymentName      string
	Static              bool
	DeploymentYAMLBytes []byte
}
