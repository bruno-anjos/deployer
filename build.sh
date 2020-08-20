#!/bin/bash

rm alternatives.txt
set -e

env CGO_ENABLED=0 GOOS=linux go build -o deployer .
docker build -t brunoanjos/deployer:latest .
touch alternatives.txt
