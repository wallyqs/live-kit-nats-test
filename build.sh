#!/usr/bin/env bash

VERSION=1

docker build . -t livekit/nats-test:$VERSION --platform linux/amd64
docker push livekit/nats-test:$VERSION
