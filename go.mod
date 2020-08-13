module github.com/bruno-anjos/deployer

go 1.13

require (
	github.com/bruno-anjos/archimedes v0.0.2
	github.com/bruno-anjos/scheduler v0.0.1
	github.com/bruno-anjos/solution-utils v0.0.1
	github.com/docker/go-connections v0.4.0
	github.com/google/uuid v1.1.1
	github.com/sirupsen/logrus v1.6.0
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
)

replace (
	github.com/bruno-anjos/archimedes v0.0.2 => ./../archimedes
	github.com/bruno-anjos/scheduler v0.0.1 => ./../scheduler
	github.com/bruno-anjos/solution-utils v0.0.1 => ./../solution-utils
)
