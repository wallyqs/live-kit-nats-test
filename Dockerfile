FROM golang:1.19-alpine as builder

ARG TARGETPLATFORM
ARG TARGETARCH
ARG GITHUB_TOKEN
RUN echo building for "$TARGETPLATFORM"

WORKDIR /workspace

RUN apk add --no-cache build-base git

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN git config --global --add url."https://${GITHUB_TOKEN}@github.com/livekit".insteadOf "https://github.com/livekit"
RUN go mod download

# Copy the go source
COPY *.go .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH GO111MODULE=on go build -a -o nats-test .

FROM alpine

COPY --from=builder /workspace/nats-test /nats-test

# Run the binary.
ENTRYPOINT ["/nats-test"]
