![Build status](https://github.com/oam-dev/kubevela/workflows/E2E/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/oam-dev/kubevela)](https://goreportcard.com/report/github.com/oam-dev/kubevela)
![Docker Pulls](https://img.shields.io/docker/pulls/oamdev/vela-core)
[![codecov](https://codecov.io/gh/oam-dev/kubevela/branch/master/graph/badge.svg)](https://codecov.io/gh/oam-dev/kubevela)
[![LICENSE](https://img.shields.io/github/license/oam-dev/kubevela.svg?style=flat-square)](/LICENSE)
[![Releases](https://img.shields.io/github/release/oam-dev/kubevela/all.svg?style=flat-square)](https://github.com/oam-dev/kubevela/releases)
[![TODOs](https://img.shields.io/endpoint?url=https://api.tickgit.com/badge?repo=github.com/oam-dev/kubevela)](https://www.tickgit.com/browse?repo=github.com/oam-dev/kubevela)
[![Twitter](https://img.shields.io/twitter/url?style=social&url=https%3A%2F%2Ftwitter.com%2Foam_dev)](https://twitter.com/oam_dev)
[![Artifact HUB](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/kubevela)](https://artifacthub.io/packages/search?repo=kubevela)

![alt](docs/resources/KubeVela-03.png)

*Make shipping applications more enjoyable.*

# KubeVela

KubeVela is the platform engine to create *PaaS-like* experience on Kubernetes, in a scalable approach.

For platform builders, KubeVela serves as a framework that empowers them to create user friendly yet highly extensible platforms at ease.
In details, KubeVela relieves the pains of building such platforms by doing the following:

- Application Centric. KubeVela enforces an **application** concept as its main API and **ALL** KubeVela's capabilities serve
  for the applications' requirements only. This is achieved by adopting the [Open Application Model](https://github.com/oam-dev/spec) 
  as the core API for KubeVela.

- Extending Natively. An application in KubeVela is composed of various modularized components (named: services).
  Capabilities from Kubernetes ecosystem can be added to KubeVela as new workload types or traits through Kubernetes CRD
  registry mechanism at any time.

- Simple yet Extensible Abstraction Mechanism. KubeVela introduced a templating engine (supports [CUELang](https://github.com/cuelang/cue)
  and more) to abstract user-facing schemas from the underlying Kubernetes resources. KubeVela provides a set of
  built-in abstractions to start with and the platform builders are free to modify them at any time.
  Abstraction changes take effect at runtime, neither recompilation nor redeployment of KubeVela is required.

With KubeVela, platform builders now finally have the tooling support to design and ship any new capabilities to their
end-users with high confidence and low turn around time.

For developers, such platforms built with KubeVela will enable them to design and ship their applications to Kubernetes with minimal effort.
Instead of managing a handful infrastructure details, a simple application definition is all they need, following an
developer centric workflow that can be easily integrated with any CI/CD pipeline.

## Community

- Slack:  [CNCF Slack](https://slack.cncf.io/) #kubevela channel
- Gitter: [Discussion](https://gitter.im/oam-dev/community)

> NOTE: KubeVela is still iterating quickly. It's currently under pre-beta release.

## Installation

Installation guide is available on [this section](https://kubevela.io/#/en/install).

## Quick Start

Quick start is available on [this section](https://kubevela.io/#/en/quick-start).

## Documentation

For full documentation, please visit the KubeVela website: [https://kubevela.io](https://kubevela.io/).

## Talks and Conferences

| Engagement | Link        |
|:-----------|:------------|
| 🎤  Talks | - [KubeVela - The Modern App Delivery System in Alibaba](https://docs.google.com/presentation/d/1CWCLcsKpDQB3bBDTfdv2BZ8ilGGJv2E8L-iOA5HMrV0/edit?usp=sharing) |
| 🌎 KubeCon | - [ [NA 2020] Standardizing Cloud Native Application Delivery Across Different Clouds](https://www.youtube.com/watch?v=0yhVuBIbHcI) <br> - [ [EU 2021] Zero Pain Microservice Development and Deployment with Dapr and KubeVela](https://sched.co/iE4S) |
| 📺 Conferences | - [Dapr, Rudr, OAM: Mark Russinovich presents next gen app development & deployment](https://www.youtube.com/watch?v=eJCu6a-x9uo) <br> - [Mark Russinovich presents "The Future of Cloud Native Applications with OAM and Dapr"](https://myignite.techcommunity.microsoft.com/sessions/82059)|

## Contributing
Check out [CONTRIBUTING](./CONTRIBUTING.md) to see how to develop with KubeVela.

## Code of Conduct
KubeVela adopts [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/master/code-of-conduct.md).
