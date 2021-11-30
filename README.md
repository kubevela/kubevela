<div style="text-align: center">
  <p align="center">
    <img src="https://raw.githubusercontent.com/oam-dev/kubevela.io/main/docs/resources/KubeVela-03.png">
    <br><br>
    <i>Make shipping applications more enjoyable.</i>
  </p>
</div>

![Build status](https://github.com/oam-dev/kubevela/workflows/E2E/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/oam-dev/kubevela)](https://goreportcard.com/report/github.com/oam-dev/kubevela)
![Docker Pulls](https://img.shields.io/docker/pulls/oamdev/vela-core)
[![codecov](https://codecov.io/gh/oam-dev/kubevela/branch/master/graph/badge.svg)](https://codecov.io/gh/oam-dev/kubevela)
[![LICENSE](https://img.shields.io/github/license/oam-dev/kubevela.svg?style=flat-square)](/LICENSE)
[![Releases](https://img.shields.io/github/release/oam-dev/kubevela/all.svg?style=flat-square)](https://github.com/oam-dev/kubevela/releases)
[![TODOs](https://img.shields.io/endpoint?url=https://api.tickgit.com/badge?repo=github.com/oam-dev/kubevela)](https://www.tickgit.com/browse?repo=github.com/oam-dev/kubevela)
[![Twitter](https://img.shields.io/twitter/url?style=social&url=https%3A%2F%2Ftwitter.com%2Foam_dev)](https://twitter.com/oam_dev)
[![Artifact HUB](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/kubevela)](https://artifacthub.io/packages/search?repo=kubevela)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/4602/badge)](https://bestpractices.coreinfrastructure.org/projects/4602)

## Introduction

KubeVela is a modern application delivery platform that makes deploying and operating applications across today's hybrid, multi-cloud environments easier, faster and more reliable.

![](docs/resources/what-is-kubevela.png)

## Highlights

KubeVela practices the "render, orchestrate, deploy" workflow with below highlighted values added to existing ecosystem:

- *Application Centric* - KubeVela introduces [Open Application Model (OAM)](https://oam.dev/) as the consistent yet higher level API to capture and render a full deployment of microservices on top of hybrid environments. Placement strategy, traffic shifting and rolling update are declared at application level. No infrastructure level concern, simply deploy.

- *Programmable Workflow* - KubeVela models application delivery as DAG (Directed Acyclic Graph) and expresses it with [CUE](https://cuelang.org/) - a modern data configuration language. This allows you to design application deployment steps per needs and orchestrate them in programmable approach. No restrictions, natively extensible.

- *Infrastructure Agnostic* - KubeVela works as an application delivery control plane that is fully decoupled from runtime infrastructure. It can deploy any workload types including containers, cloud services, databases, or even VM instances to any cloud or Kubernetes cluster, following the workflow designed by you.

## Getting Started

- [Introduction](https://kubevela.io/docs)
- [Installation](https://kubevela.io/docs/install)
- [Design Your First Deployment Plan](https://kubevela.io/docs/quick-start)

## Documentation

Full documentation is available on the [KubeVela website](https://kubevela.io/).

## Community

- Slack:  [CNCF Slack](https://slack.cncf.io/) #kubevela channel (*English*)
- Gitter: [oam-dev](https://gitter.im/oam-dev/community) (*English*)
- [DingTalk Group](https://page.dingtalk.com/wow/dingtalk/act/en-home): `23310022` (*Chinese*)
- Wechat Group (*Chinese*): Broker wechat to add you into the user group.
 
  <img src="https://static.kubevela.net/images/barnett-wechat.jpg" width="200" />
- Bi-weekly Community Call: [Meeting Notes](https://docs.google.com/document/d/1nqdFEyULekyksFHtFvgvFAYE-0AMHKoS3RMnaKsarjs)

## Talks and Conferences

| Engagement | Link        |
|:-----------|:------------|
| 🎤  Talks | - [KubeVela - The Modern App Delivery System in Alibaba](https://docs.google.com/presentation/d/1CWCLcsKpDQB3bBDTfdv2BZ8ilGGJv2E8L-iOA5HMrV0/edit?usp=sharing) |
| 🌎 KubeCon | - [ [NA 2020] Standardizing Cloud Native Application Delivery Across Different Clouds](https://www.youtube.com/watch?v=0yhVuBIbHcI) <br> - [ [EU 2021] Zero Pain Microservice Development and Deployment with Dapr and KubeVela](https://sched.co/iE4S) |
| 📺 Conferences | - [Dapr, Rudr, OAM: Mark Russinovich presents next gen app development & deployment](https://www.youtube.com/watch?v=eJCu6a-x9uo) <br> - [Mark Russinovich presents "The Future of Cloud Native Applications with OAM and Dapr"](https://myignite.techcommunity.microsoft.com/sessions/82059)|

## Contributing
Check out [CONTRIBUTING](./CONTRIBUTING.md) to see how to develop with KubeVela.

## Report Vulnerability

Security is a first priority thing for us at KubeVela. If you come across a related issue, please send email to security@mail.kubevela.io .

## Code of Conduct
KubeVela adopts [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/master/code-of-conduct.md).
