---
title:  Roadmap
---

Date: 2021-04-01 to 2021-06-30

## Core Platform

1. Implement Application serverside Kustomize and Workflow.
2. KubeVela as a control plane.
    - Application Controller deploy resources directly to remote clusters and instead of using AppContext
    - AppRollout should be able to work in runtime cluster or rollout remote cluster resources
3. Multi-cluster and Multi-environment support, applications can deploy in different environments which
   contains different clusters with different strategies.
4. Better Helm and Kustomize support, users can deploy a helm chart or a git repo directly without any more effort.
5. Support built-in Application monitoring.
6. Support more rollout strategies.
    - blue-green
    - traffic management rollout
    - canary
    - A/B
7. Support a general CUE controller which can glue more than K8s CRDs, it should support more protocol such as restful API,
   go function call, etc.
8. Discoverable capability registries with more back integrations(file server/github/OSS).

## User Experience

1. Develop tools and CI integration.
2. Refine our docs and website.

## Third-party integrations

1. Integrate with Open Cluster Management.
2. Integrate with Flux CD
3. Integrate with Dapr to enable end-to-end microservice application development and deployment workflow.
4. Integrate with Tilt for local development.
