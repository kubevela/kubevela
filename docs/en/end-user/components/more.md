---
title:  Want More?
---

Components in KubeVela are designed to be brought by users.

Check below documentations about how to bring your own components to the system in various approaches.

- [Helm](../../platform-engineers/helm/component) - Helm chart is a natural form of component, note that you need to have a valid Helm repository (e.g. GitHub repo or a Helm hub) to host the chart in this case.
- [CUE](../../platform-engineers/cue/component) - CUE is powerful approach to encapsulate a component and it doesn't require any repository.
- [Simple Template](../../platform-engineers/kube/component) - Not a Helm or CUE expert? A simple template approach is also provided to define any Kubernetes API resource as a component. Note that only key-value style parameters are supported in this case.
- [Cloud Services](../../platform-engineers/cloud-services) - KubeVela allows you to declare cloud services as part of the application and provision them in consistent API.
