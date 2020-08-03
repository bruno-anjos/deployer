package main

type DeploymentYAML struct {
	Spec struct {
		Replicas    int
		ServiceName string
		Template struct{
			Spect struct{
				Containers struct{
					
				}
			}
		}
	}
}

type Deployment struct {
	DeploymentName    string
	NumberOfInstances int
	EnvVars           string
}
