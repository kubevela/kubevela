SHELL := /bin/bash

GOBUILD_ENV = GO111MODULE=on CGO_ENABLED=0
GOX         = go run github.com/mitchellh/gox
TARGETS     := darwin/amd64 linux/amd64 windows/amd64
DIST_DIRS   := find * -type d -exec

TIME_LONG	= `date +%Y-%m-%d' '%H:%M:%S`
TIME_SHORT	= `date +%H:%M:%S`
TIME		= $(TIME_SHORT)

BLUE         := $(shell printf "\033[34m")
YELLOW       := $(shell printf "\033[33m")
RED          := $(shell printf "\033[31m")
GREEN        := $(shell printf "\033[32m")
CNone        := $(shell printf "\033[0m")

INFO	= echo ${TIME} ${BLUE}[INFO]${CNone}
WARN	= echo ${TIME} ${YELLOW}[WARN]${CNone}
ERR		= echo ${TIME} ${RED}[FAIL]${CNone}
OK		= echo ${TIME} ${GREEN}[ OK ]${CNone}
FAIL	= (echo ${TIME} ${RED}[FAIL]${CNone} && false)



# Vela version
VELA_VERSION ?= master
# Repo info
GIT_COMMIT          ?= git-$(shell git rev-parse --short HEAD)
GIT_COMMIT_LONG     ?= $(shell git rev-parse HEAD)
VELA_VERSION_KEY    := github.com/oam-dev/kubevela/version.VelaVersion
VELA_GITVERSION_KEY := github.com/oam-dev/kubevela/version.GitRevision
LDFLAGS             ?= "-s -w -X $(VELA_VERSION_KEY)=$(VELA_VERSION) -X $(VELA_GITVERSION_KEY)=$(GIT_COMMIT)"



# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Image URL to use all building/pushing image targets
VELA_CORE_IMAGE      ?= vela-core:latest
VELA_CLI_IMAGE      ?= oamdev/vela-cli:latest
VELA_CORE_TEST_IMAGE ?= vela-core-test:$(GIT_COMMIT)
VELA_APISERVER_IMAGE      ?= apiserver:latest
VELA_RUNTIME_ROLLOUT_IMAGE       ?= vela-runtime-rollout:latest
VELA_RUNTIME_ROLLOUT_TEST_IMAGE  ?= vela-runtime-rollout-test:$(GIT_COMMIT)
RUNTIME_CLUSTER_CONFIG ?= /tmp/worker.client.kubeconfig
RUNTIME_CLUSTER_NAME ?= worker