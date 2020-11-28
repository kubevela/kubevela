# Build the manager binary
FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.14 as builder

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

# Build
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} GO111MODULE=on go build -a -o manager-${TARGETARCH} main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
# Could use `--build-arg=BASE_DISTROLESS=gcr.io/distroless/static:nonroot` to overwrite
ARG BASE_DISTROLESS
FROM ${BASE_DISTROLESS:-gcr.io/distroless/static:nonroot}

WORKDIR /

ARG TARGETARCH
COPY --from=builder /workspace/manager-${TARGETARCH} /manager
USER nonroot:nonroot

ENTRYPOINT ["/manager"]
