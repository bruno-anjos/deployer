module github.com/bruno-anjos/deployer

go 1.13

require (
	github.com/bruno-anjos/archimedes v0.0.0-20200803163701-2d9c69b22560
	github.com/bruno-anjos/scheduler v0.0.0-20200804140215-71b908c75919
	github.com/bruno-anjos/solution-utils v0.0.0-20200804140242-989a419bda22
	github.com/docker/go-connections v0.4.0
	github.com/sirupsen/logrus v1.6.0
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
)

replace (
	github.com/bruno-anjos/archimedes v0.0.0-20200803163701-2d9c69b22560 => ./../archimedes
	github.com/bruno-anjos/scheduler v0.0.0-20200803172400-74b1d18055fd => ./../scheduler
	github.com/bruno-anjos/solution-utils v0.0.0-20200803160423-4cf841cde3d3 => ./../solution-utils
)
