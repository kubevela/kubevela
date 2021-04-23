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

*Developers simply want to deploy.*

Traditional *Platform-as-a-Service (PaaS)* systems enable easy application deployments, but this happiness disappears when your application outgrows the capabilities of your platform. This is inevitable regardless of your PaaS is built on Kubernetes or not - the root cause is its inflexibility.

KubeVela is a modern application platform that is fully self-service, and adapts to your needs when you grow.

Leveraging Kubernetes as control plane, KubeVela itself is runtime agnostic. It allows you to deploy (and manage) containerized workloads, cloud functions, databases, or even EC2 instances with a consistent workflow.

## Features

**Developer Centric** - KubeVela introduces higher level API to capture a full deployment of microservices, and builds features around the application needs only. Progressive rollout and multi-cluster deployment are provided out-of-box. No infrastructure level concerns, simply deploy.

**Self-service** - KubeVela models platform features (such as workloads, operational behaviors, and cloud services) as reusable [CUE](https://github.com/cuelang/cue) and/or [Helm](https://helm.sh/) components, and expose them to end users as self-service building blocks. When your needs grow, these capabilities can extend naturally in a programmable approach. No restriction, fully flexible.

**Simple yet Reliable** - KubeVela is built with Kubernetes as control plane so unlike traditional X-as-Code solutions, it never leaves configuration drift in your clusters. Also, this makes KubeVela work with any CI/CD or GitOps tools via declarative API without any integration burden.

## Getting Started

- [Installation](https://kubevela.io/docs/install)
- [Quick start](https://kubevela.io/docs/quick-start)
- [How it works](https://kubevela.io/docs/concepts)

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
