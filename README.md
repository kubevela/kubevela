![Build status](https://github.com/oam-dev/kubevela/workflows/E2E/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/oam-dev/kubevela)](https://goreportcard.com/report/github.com/oam-dev/kubevela)
![Docker Pulls](https://img.shields.io/docker/pulls/oamdev/vela-core)
[![codecov](https://codecov.io/gh/oam-dev/kubevela/branch/master/graph/badge.svg)](https://codecov.io/gh/oam-dev/kubevela)
[![LICENSE](https://img.shields.io/github/license/oam-dev/kubevela.svg?style=flat-square)](/LICENSE)
[![Releases](https://img.shields.io/github/release/oam-dev/kubevela/all.svg?style=flat-square)](https://github.com/oam-dev/kubevela/releases)
[![TODOs](https://img.shields.io/endpoint?url=https://api.tickgit.com/badge?repo=github.com/oam-dev/kubevela)](https://www.tickgit.com/browse?repo=github.com/oam-dev/kubevela)
[![Twitter](https://img.shields.io/twitter/url?style=social&url=https%3A%2F%2Ftwitter.com%2Foam_dev)](https://twitter.com/oam_dev)
[![Artifact HUB](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/kubevela)](https://artifacthub.io/packages/search?repo=kubevela)

![alt](docs/en/resources/KubeVela-03.png)

*Make shipping applications more enjoyable.*

# KubeVela

KubeVela is a modern application platform that makes deploying and managing applications across today's hybrid, multi-cloud environments easier and faster.

## Features

**Application Centric** - Leveraging Open Application Model (OAM), KubeVela introduces consistent yet higher level API to capture a full deployment of microservices on top of hybrid environments. Placement strategy and patch, traffic shifting and rolling update are all declared at application level. No infrastructure level concern, simply deploy.

**Natively Extensible** - KubeVela uses [CUE](https://github.com/cuelang/cue) to glue capabilities (e.g. workload types, operational behaviors, and cloud services) provided by runtime infrastructure and expose them to users via self-service API. When users' needs grow, these API can naturally expand in programmable approach. No restriction, fully flexible.

**Runtime Agnostic** - KubeVela is built with Kubernetes as control plane but adaptable to any runtime as data-plane. It can deploy (and manage) diverse workload types such as container, cloud functions, databases, or even EC2 instances across hybrid environments. Also, this means KubeVela seamlessly works with any Kubernetes compatible CI/CD or GitOps tools via declarative API.

## Getting Started

- [Introduction](https://kubevela.io/docs)
- [Installation](https://kubevela.io/docs/install)
- [Deploy an Application](https://kubevela.io/docs/application)

## Documentation

Full documentation is available on the [KubeVela website](https://kubevela.io/).

## Community

- Slack:  [CNCF Slack](https://slack.cncf.io/) #kubevela channel (*English*)
- Gitter: [oam-dev](https://gitter.im/oam-dev/community) (*English*)
- [DingTalk Group](https://page.dingtalk.com/wow/dingtalk/act/en-home): `23310022` (*Chinese*)
- Bi-weekly Community Call: [Meeting Notes](https://docs.google.com/document/d/1nqdFEyULekyksFHtFvgvFAYE-0AMHKoS3RMnaKsarjs)

## Talks and Conferences

| Engagement | Link        |
|:-----------|:------------|
| 🎤  Talks | - [KubeVela - The Modern App Delivery System in Alibaba](https://docs.google.com/presentation/d/1CWCLcsKpDQB3bBDTfdv2BZ8ilGGJv2E8L-iOA5HMrV0/edit?usp=sharing) <br> - [Cloud-Native Apps With Open Application Model (OAM) And KubeVela](https://www.youtube.com/watch?v=2CBu6sOTtwk)  |
| 🌎 KubeCon | - [ [NA 2020] Standardizing Cloud Native Application Delivery Across Different Clouds](https://www.youtube.com/watch?v=0yhVuBIbHcI) <br> - [ [EU 2021] Zero Pain Microservice Development and Deployment with Dapr and KubeVela](https://sched.co/iE4S) |
| 📺 Conferences | - [Dapr, Rudr, OAM: Mark Russinovich presents next gen app development & deployment](https://www.youtube.com/watch?v=eJCu6a-x9uo) <br> - [Mark Russinovich presents "The Future of Cloud Native Applications with OAM and Dapr"](https://myignite.techcommunity.microsoft.com/sessions/82059)|

## Contributing
Check out [CONTRIBUTING](./CONTRIBUTING.md) to see how to develop with KubeVela.

## Code of Conduct
KubeVela adopts [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/master/code-of-conduct.md).
