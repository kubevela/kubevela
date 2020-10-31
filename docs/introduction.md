# Introduction to KubeVela

![alt](../resources/KubeVela-01.png)

## Motivation

The modern trends of cloud-native application management have moved towards
pursuing a simplified developer experience across clouds and infrastructures.
As the dominating cloud-native platform, Kubernetes, excellently 
abstracts the infrastructure details and manages application lifecycle in
a consistent manner. However, many application developers
believe that Kubernete is not an easy platform to use. The primary reason
is that Kuberntes exposes full levels of API details in order to enable 
flexible users control. Despite the flexibility, it leads to a few problems.

Firstly, application developers have to understand quite a few new 
concepts and resource specifications before deploying their applications.
Sometimes, this is not an easy task. For example, it may take a while
for developers to understand some runtime related configurations
such as `allowPrivilegeEscalation` which they rarely touch in daily work.
The pervasive use of CRDs makes the situation even worse since developers
have to learn all the CRD schemas as well. 

Moreover, since in Kubernetes developers have to know all of 
the configuration details of involved objects, application management soon
becomes a headache of handling a large amount of resource YAML files.
We have seen that the lack of API abstraction had led to low productivity,
unexpected errors or misconfigurations in production. Application developers
start to question "why am I bothered with these many YAML files?"

On the other hand, Kubernetes is a platform for platforms. Literally, the extended
capabilities provided by the platform builders are the keys to support the
Kubenetes ecosystems. Nowadays, a typical production Kubernetes would install
dozens of customized operators and plugins. Since abstracting the Kubernetes
APIs is a highly opinionated process, the resultant simplified interfaces
would only make sense had the decision makers been the platform builders.
Unfortunately, the platform builders today face the following dilemmas when
dealing with the API abstraction:

- There is no tool or framework for them to easily extend the API abstraction
  if any. They have to rely on existing interfaces or assumptions for new
  capabilities which may be only suitable for specific user cases.
  This makes extending such platform to leverage broader Kubernetes ecosystems
  almost impossible.

- It could be painful to develop new capability based on existing interfaces.
  The operator design could easily becomes a marathon negotiation in order
  to meet the abstraction requirement.

In the end, application developers may complain that Kubernetes is hard to use
and the platform builders may want to help but they cannot do it easily.

## What is KubeVela?

KubeVela is a framework to help platform builders to easily expose and extend
Kubernetes capabilities for application management. It is built on top of
Kubernetes and relieves the pains of both the platform builders and the application
developers by doing the following:

- KubeVela enforces a single **application** concept and **ALL** the exposed
  Kubernetes capabilities serve for the applications only.
  It adopts the [Open Application Model](https://github.com/oam-dev/spec) (OAM)
  for its application definition. The conventional Pod and container concepts
  are completely eliminated.
 
- A KubeVela application is composed of various supported components or
  [traits](https://github.com/oam-dev/spec/blob/master/introduction.md) and their
  schemas are determined by the platform builders. New capabilities can be added
  to KubeVela through a CRD registry mechanism. 

- KubeVela provides a rich set of tools to help platform builders to abstract
  the Kubernetes resource and CRD APIs.
  For example, a [CUELang](https://github.com/cuelang/cue) based templating tool
  is used to easily build the contract between the user-facing schemas and the
  underline Kubernetes Objects. KubeVela provides a set of built-in CUE templates
  for platform builders to start with.

- Schema changes take effect immediately. Neither recompiliation nor redeployment
  of KubeVela is required. This makes the process of delivering new capabilities
  using KubeVela extremely simple.
  
With KubeVela, platform builders now have indefinite flexibilities in designing
and implementing new capabilities without worrying about what and how to expose
the new capabilities to the end users. However, the ultimate beneficiaries are
the application developers:
- Instead of managing a handful Kubernetes YAML files, only a simple
  docker-compose style [appfile](./docs/developers/devex/appfile.md) is needed
  to manage an application in KubeVela. 
- Out of the box, KubeVela provides a CLI tool to simplify the application
  management workflow which can be easily integrated with existing CI/CD pipelines.

## Comparisons

### Platform-as-a-Service (PaaS) 

The typical examples are Heroku and Cloud Foundry. They also provide full
application management capabilities and aim to improve developer experience
and efficiency. KubeVela shares the same goal but its built-in features are
much lighter and easier to maintain compared to most of the existing PaaS offerings.
KubeVela core components are nothing but a set of Kubernetes controllers/plugins.

The biggest difference lies in the extensibility. Most PaaS systems enforce
constraints in the type of supported applications and the supported capabilities.
They are either inextensible or create their own addon systems maintained by the
user communities. In contrast, KubeVela is built on top of Kubernetes,
and all the supported capabilities are implemented by Kubernetes CRD controllers.
No additional addon system is introduced. A new capability can be installed in
KubeVela at any time by simply registering the CRD in KubeVela.


### Serverless platforms  

Serverless platform such as AWS Lambda provides extraordinary user experience
and agility to deploy serverless applications. However, those platforms impose
even more constraints in extensibility. They are argurably "hard-coded" PaaS.

Kubernetes based serverless platforms such as Knative, OpenFaaS can be easily
integrated with KubeVela by registering themself as platform capabilities.
Even for AWS Lambda, there is an success story to integrate it with KubeVela
by the tools developed by Crossplane.

### Platform agnostic developer tools

The typical example is [Waypoint](https://github.com/hashicorp/waypoint). It is
a user facing tool which introduces a consistent workflow (i.e., build,
deploy, release) for developers to ship applications on top of different platforms.
KubeVela can be integrated into Waypoint like any other supported platforms. 
In this case, developers can use the Waypoint workflow instead of the KubeVela
CLI to manage applications.


### Package management tools 

People may mistakenly think KubeVela is another package manager like Helm.
Although using Helm chart significantly reduces the burden of managing a
complicated Kubernetes application, whoever prepares the helm chart cannot avoid
the tedious and error-prone work of packaging those YAML files.

KubeVela aims to fundamentally remove the need of managing conventional Kubernetes
YAML files. However, in the server side, KubeVela still relies on Helm to package
and manage the third-party plugins such as `Prometheus`, etc.

### Kubernetes

KubeVela can be treated as a special Kubernetes "distribution" that focuses on
developers and application management only.
It leverages the native Kubernetes extensibility to resolve a hard problem - making
application management enjoyable in Kubernetes.
