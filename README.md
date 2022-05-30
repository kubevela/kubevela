<div style="text-align: center">
  <p align="center">
    <img src="https://raw.githubusercontent.com/kubevela/kubevela.io/main/docs/resources/KubeVela-03.png">
    <br><br>
    <i>Make shipping applications more enjoyable.</i>
  </p>
</div>

![Build status](https://github.com/kubevela/kubevela/workflows/E2E/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/kubevela/kubevela)](https://goreportcard.com/report/github.com/kubevela/kubevela)
![Docker Pulls](https://img.shields.io/docker/pulls/oamdev/vela-core)
[![codecov](https://codecov.io/gh/kubevela/kubevela/branch/master/graph/badge.svg)](https://codecov.io/gh/kubevela/kubevela)
[![LICENSE](https://img.shields.io/github/license/kubevela/kubevela.svg?style=flat-square)](/LICENSE)
[![Releases](https://img.shields.io/github/release/kubevela/kubevela/all.svg?style=flat-square)](https://github.com/kubevela/kubevela/releases)
[![TODOs](https://img.shields.io/endpoint?url=https://api.tickgit.com/badge?repo=github.com/kubevela/kubevela)](https://www.tickgit.com/browse?repo=github.com/kubevela/kubevela)
[![Twitter](https://img.shields.io/twitter/url?style=social&url=https%3A%2F%2Ftwitter.com%2Foam_dev)](https://twitter.com/oam_dev)
[![Artifact HUB](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/kubevela)](https://artifacthub.io/packages/search?repo=kubevela)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/4602/badge)](https://bestpractices.coreinfrastructure.org/projects/4602)

## Introduction

KubeVela is a modern application delivery platform that makes deploying and operating applications across today's hybrid, multi-cloud environments easier, faster and more reliable.

![kubevela](docs/resources/what-is-kubevela.png)

## Key Features

KubeVela practices the "render, orchestrate, deploy" workflow with below highlighted values added to existing ecosystem:

* Deployment as Code

Declare your deployment plan as workflow, run it automatically with any CI/CD or GitOps system, extend or re-program the workflow steps with CUE. No add-hoc scripts, no dirty glue code, just deploy. The deployment workflow in KubeVela is powered by [Open Application Model](https://oam.dev/).

* Built-in security and compliance building blocks

Choose from the wide range of LDAP integrations we provided out-of-box, enjoy multi-cluster authorization that is fully automated, pick and apply fine-grained RBAC modules and customize them per your own supply chain requirements.

* Multi-cloud/hybrid-environments app delivery as first-class citizen

Progressive rollout across test/staging/production environments, automatic canary, blue-green and continuous verification, rich placement strategy across clusters and clouds, fully managed cloud environments provision.

## Getting Started

We provide a demo environment for you quickly experience:

Demo Address: [http://demo.kubevela.net](http://demo.kubevela.net)

Demo Account(username/password): demo / KubeVela666

> This account has limited permissions, to get full capabilities please install your own environment.

* [Introduction](https://kubevela.io/docs)
* [Installation](https://kubevela.io/docs/install)
* [Deploy Your Application](https://kubevela.io/docs/quick-start)

## Documentation

Full documentation is available on the [KubeVela website](https://kubevela.io/).

## Blog

Official blog is available on [KubeVela blog](https://kubevela.io/blog).

## Community

We want your contributions and suggestions! 
One of the easiest ways to contribute is to participate in discussions on the Github Issues/Discussion, chat on IM or the bi-weekly community calls.
For more information on the community engagement, developer and contributing guidelines and more, head over to the [KubeVela community repo](https://github.com/kubevela/community).

### Contact Us

Reach out with any questions you may have and we'll make sure to answer them as soon as possible!

- Slack:  [CNCF Slack kubevela channel](https://cloud-native.slack.com/archives/C01BLQ3HTJA) (*English*)
- [DingTalk Group](https://page.dingtalk.com/wow/dingtalk/act/en-home): `23310022` (*Chinese*)
- Wechat Group (*Chinese*): Broker wechat to add you into the user group.
 
  <img src="https://static.kubevela.net/images/barnett-wechat.jpg" width="200" />

### Community Call

Every two weeks we host a community call to showcase new features, review upcoming milestones, and engage in a Q&A. All are welcome!

- Bi-weekly Community Call:
  - [Meeting Notes](https://docs.google.com/document/d/1nqdFEyULekyksFHtFvgvFAYE-0AMHKoS3RMnaKsarjs).
  - [Video Records](https://kubevela.io/videos/meetings/en/meetings).
- Bi-weekly Chinese Community Call:
  - [Video Records](https://kubevela.io/videos/meetings/cn/v1.3).

## Talks and Conferences

| Engagement | Link        |
|:-----------|:------------|
| 🎤  Talks | - [KubeVela - The Modern App Delivery System in Alibaba](https://docs.google.com/presentation/d/1CWCLcsKpDQB3bBDTfdv2BZ8ilGGJv2E8L-iOA5HMrV0/edit?usp=sharing) |
| 🌎 KubeCon | - [ [NA 2020] Standardizing Cloud Native Application Delivery Across Different Clouds](https://www.youtube.com/watch?v=0yhVuBIbHcI) <br> - [ [EU 2021] Zero Pain Microservice Development and Deployment with Dapr and KubeVela](https://sched.co/iE4S) |
| 📺 Conferences | - [Dapr, Rudr, OAM: Mark Russinovich presents next gen app development & deployment](https://www.youtube.com/watch?v=eJCu6a-x9uo) <br> - [Mark Russinovich presents "The Future of Cloud Native Applications with OAM and Dapr"](https://myignite.techcommunity.microsoft.com/sessions/82059)|

For more talks, please checkout [KubeVela Talks](https://kubevela.io/videos/talks/en/standardizing-app).

## Contributing

Check out [CONTRIBUTING](./CONTRIBUTING.md) to see how to develop with KubeVela.

## Report Vulnerability

Security is a first priority thing for us at KubeVela. If you come across a related issue, please send email to security@mail.kubevela.io .

## Code of Conduct

KubeVela adopts [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/master/code-of-conduct.md).
