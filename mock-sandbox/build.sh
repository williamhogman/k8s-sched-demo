#!/usr/bin/env bash

export DOCKER_HOST=$(docker context inspect | jq '.[0].Endpoints.docker.Host')
export KO_DOCKER_REPO=ko.local/sandbox

ko build --bare --tags=latest ./cmd/sandbox