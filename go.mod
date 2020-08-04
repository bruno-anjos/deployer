module github.com/bruno-anjos/deployer

go 1.13

require (
	github.com/bruno-anjos/archimedes v0.0.0-20200804153633-d07ca32d62f3
	github.com/bruno-anjos/scheduler v0.0.0-20200804151330-da4916073155
	github.com/bruno-anjos/solution-utils v0.0.0-20200804140242-989a419bda22
	github.com/docker/go-connections v0.4.0
	github.com/sirupsen/logrus v1.6.0
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
)

replace (
	github.com/bruno-anjos/archimedes v0.0.0-20200804153633-d07ca32d62f3 => ./../archimedes
	github.com/bruno-anjos/scheduler v0.0.0-20200804151330-da4916073155 => ./../scheduler
	github.com/bruno-anjos/solution-utils v0.0.0-20200804140242-989a419bda22 => ./../solution-utils
)
