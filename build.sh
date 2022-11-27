#!/usr/bin/env bash

VERSION=6

docker build . -t thedavidzhao/nats-test:$VERSION --platform linux/amd64 --build-arg GITHUB_TOKEN=$GITHUB_TOKEN
docker push thedavidzhao/nats-test:$VERSION
