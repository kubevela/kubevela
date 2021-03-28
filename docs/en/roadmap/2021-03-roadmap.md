---
title:  Roadmap
---

Date: 2021-01-01 to 2021-03-30

## Core Platform

- Add Application object as the deployment unit applied to k8s control plane.
  - The new Application object will handle CUE template rendering on the server side. So the appfile would be translated to Application object directly without doing client side rendering.
  - CLI/UI will be updated to replace ApplicationConfiguration and Component objects with Application object.
- Integrate Terraform as one of the core templating engines so that platform builders can add Terraform modules as Workloads/Traits into KubeVela.
- Re-architect API Server to have clean API and storage layer as [designed](https://github.com/oam-dev/kubevela/blob/master/design/vela-core/APIServer-Catalog.md#2-api-design).
- Automatically sync Catalog server and display packages information as [designed](https://github.com/oam-dev/kubevela/blob/master/design/vela-core/APIServer-Catalog.md#3-catalog-design).
- Add Rollout CRD to do native Workload and Application level application rollout management.
- Support intermediate store (e.g. ConfigMap) and JSON patch operations in data input/output.

## User Experience

- Rewrite dashboard to support up-to-date Vela object model.
  - Support dynamic form rendering based on OpenAPI schema generated from Definition objects.
  - Support displaying pages of applications, capabilities, catalogs.
- Automatically generate reference docs for capabilities and support displaying them in CLI/UI devtools.

## Third-party integrations

- Integrate with S2I (Source2Image) tooling like [Derrick](https://github.com/alibaba/derrick) to enable more developer-friendly workflow in appfile.
- Integrate with Dapr to enable end-to-end microservice application development and deployment workflow.
