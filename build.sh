#!/bin/bash

set -e

env CGO_ENABLED=0 GOOS=linux go build -o deployer .
docker build -t brunoanjos/deployer:latest .
docker push brunoanjos/deployer:latest