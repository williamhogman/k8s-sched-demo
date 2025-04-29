#!/usr/bin/env bash

export DOCKER_HOST=unix:///Users/whn/.rd/docker.sock
export KO_DOCKER_REPO=ko.local/sandbox

ko build --bare --tags=latest ./cmd/sandbox