# Build the manager binary
FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.16 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/core/main.go main.go
COPY apis/ apis/
COPY pkg/ pkg/
COPY version/ version/

# Build
ARG TARGETARCH
ARG VERSION
ARG GITVERSION
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} GO111MODULE=on \
    go build -a -ldflags "-X github.com/oam-dev/kubevela/version.VelaVersion=${VERSION:-undefined} -X github.com/oam-dev/kubevela/version.GitRevision=${GITVERSION:-undefined}" \
    -o manager-${TARGETARCH} main.go

# Use ubuntu as base image for convenience.
# You can replace distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
# Could use `--build-arg=BASE_IMAGE=gcr.io/distroless/static:nonroot` to overwrite
ARG BASE_IMAGE
FROM ${BASE_IMAGE:-ubuntu:latest}
# This is required by daemon connnecting with cri
RUN ln -s /usr/bin/* /usr/sbin/ && apt-get update -y \
  && apt-get install --no-install-recommends -y ca-certificates \
  && apt-get clean && rm -rf /var/log/*log /var/lib/apt/lists/* /var/log/apt/* /var/lib/dpkg/*-old /var/cache/debconf/*-old

WORKDIR /

ARG TARGETARCH
COPY --from=builder /workspace/manager-${TARGETARCH} /manager

ENTRYPOINT ["/manager"]
