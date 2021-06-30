---
title: Overview
---

Cloud services are important components of your application, and KubeVela allows you to provision and consume them in a consistent experience.

## How Does KubeVela Manage Cloud Services?

In KubeVela, the needed cloud services are claimed as *components* in an application, and consumed via *Service Binding Trait* by other components.

## Does KubeVela Talk to the Clouds?

KubeVela relies on [Terraform Controller](https://github.com/oam-dev/terraform-controller) or [Crossplane](http://crossplane.io/) as providers to talk to the clouds. Please check the documentations below for detailed steps.

- [Terraform](./terraform)
- [Crossplane](./crossplane)

## Can a Instance of Cloud Services be Shared by Multiple Applications?

Yes. Though we currently defer this to providers so by default the cloud service instances are not shared and dedicated per `Application`. A workaround for now is you could use a separate `Application` to declare the cloud service only, then other `Application` can consume it via service binding trait in a shared approach.

In the future, we are considering making this part as a standard feature of KubeVela so you could claim whether a given cloud service component should be shared or not.
