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

KubeVela is a modern application engine that adapts to your application's needs, not the other way around.

## Community

- Slack:  [CNCF Slack](https://slack.cncf.io/) #kubevela channel
- Gitter: [Discussion](https://gitter.im/oam-dev/community)
- Bi-weekly Community Call: [Meeting Notes](https://docs.google.com/document/d/1nqdFEyULekyksFHtFvgvFAYE-0AMHKoS3RMnaKsarjs)

## What problems does it solve?

Traditional Platform-as-a-Service (PaaS) system enables easy application deployments and everything just works, but this happiness disappears when your application outgrows the capabilities of your platform.

KubeVela is a modern application engine whose capabilities are actually Infrastructure-as-Code (IaC) components coded by you or come from the ecosystem. Think of it as a *Heroku* which is fully programmable to serve your needs as you grow and expand.

As a plus, KubeVela leverages [Kubernetes Control Loop](https://kubernetes.io/docs/concepts/architecture/controller/) to enforce all those abstractions so they will never leave *configuration drift* (i.e. the running instances are not in line with the expected configuration) in your clusters.

## Getting Started

- [Installation](https://kubevela.io/docs/install)
- [Quick start](https://kubevela.io/docs/quick-start)
- [How it works](https://kubevela.io/docs/concepts)

## Features

- **Zero-restriction deployment** - design and express platform capabilities with [CUE](https://cuelang.org/) or [Helm](https://helm.sh) per needs of your application, and let Kubernetes controller guarantee the deployment determinism. GUI forms are automatically generated for capabilities so even your dashboard are fully extensible.
- **Generic progressive rollout framework** - built-in rollout framework and strategies to upgrade your microservice regardless of its workload type (e.g. stateless, stateful, or even custom operators etc).
- **Multi-cluster multi-revision application deployment** - built-in model to deploy or rollout your apps across hybrid infrastructures, with Service Mesh for traffic shifting. 
- **Simple and native** - KubeVela is a just simple Kubernetes custom controller, all its capabilities are defined as [Custom Resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) so they naturally work with any CI/CD or GitOps tools that works with Kubernetes.

## Documentation

Visit the [KubeVela documentation site](https://kubevela.io/) to find *Installation Instruction*, *Platform Builder Guide* and *Developer Experience Guide*.

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
