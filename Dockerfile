ARG BASE_IMAGE
# Build the manager binary
FROM golang:1.23.8-alpine@sha256:b7486658b87d34ecf95125e5b97e8dfe86c21f712aa36fc0c702e5dc41dc63e1 AS builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# It's a proxy for CN developer, please unblock it if you have network issue
ARG GOPROXY
ENV GOPROXY=${GOPROXY:-https://proxy.golang.org}

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source for building core
COPY cmd/core/ cmd/core/
COPY apis/ apis/
COPY pkg/ pkg/
COPY version/ version/
COPY references/ references/

# Build
ARG TARGETARCH
ARG VERSION
ARG GITVERSION
RUN GO111MODULE=on CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -a -ldflags "-s -w -X github.com/oam-dev/kubevela/version.VelaVersion=${VERSION:-undefined} -X github.com/oam-dev/kubevela/version.GitRevision=${GITVERSION:-undefined}" \
    -o manager-${TARGETARCH} cmd/core/main.go

# Use alpine as base image due to the discussion in issue #1448
# You can replace distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
# Overwrite `BASE_IMAGE` by passing `--build-arg=BASE_IMAGE=gcr.io/distroless/static:nonroot`
FROM ${BASE_IMAGE:-alpine:3.18}
# This is required by daemon connecting with cri
RUN apk add --no-cache ca-certificates bash expat

WORKDIR /

ARG TARGETARCH
COPY --from=builder /workspace/manager-${TARGETARCH} /usr/local/bin/manager

COPY entrypoint.sh /usr/local/bin/

ENTRYPOINT ["entrypoint.sh"]

CMD ["manager"]
