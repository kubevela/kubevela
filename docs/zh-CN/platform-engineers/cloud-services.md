---
title: 概述
---

云服务是应用程序的重要组件，KubeVela 允许您以一致的体验配置和使用它们。

## KubeVela 如何管理云服务？

在 KubeVela 中，所需的云服务在应用程序中被声明为*components*，并通过*Service Binding Trait*被其他组件使用。

## KubeVela 是否与云对话？

KubeVela 依靠 [Terraform Controller](https://github.com/oam-dev/terraform-controller) 或 [Crossplane](http://crossplane.io/) 作为提供者与云对话。请查看以下文档以了解详细步骤。

- [Terraform](./terraform)
- [Crossplane](./crossplane)

## 一个云服务实例可以被多个应用程序共享吗？

是的。虽然我们目前将此推迟到提供者，因此默认情况下，云服务实例不是每个“应用程序”共享和专用的。现在的解决方法是您可以使用单独的“应用程序”仅声明云服务，然后其他“应用程序”可以通过共享方法中的 service binding trait 来使用它。

将来，我们正在考虑将此部分作为 KubeVela 的标准功能，以便您可以声明是否应共享给定的云服务组件。