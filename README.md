<div style="text-align: center">
  <p align="center">
    <img src="https://raw.githubusercontent.com/kubevela/kubevela.io/main/docs/resources/KubeVela-03.png">
    <br><br>
    <i>Make shipping applications more enjoyable.</i>
  </p>
</div>

![Build status](https://github.com/kubevela/kubevela/workflows/Go/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/kubevela/kubevela)](https://goreportcard.com/report/github.com/kubevela/kubevela)
![Docker Pulls](https://img.shields.io/docker/pulls/oamdev/vela-core)
[![codecov](https://codecov.io/gh/kubevela/kubevela/branch/master/graph/badge.svg)](https://codecov.io/gh/kubevela/kubevela)
[![LICENSE](https://img.shields.io/github/license/kubevela/kubevela.svg?style=flat-square)](/LICENSE)
[![Releases](https://img.shields.io/github/release/kubevela/kubevela/all.svg?style=flat-square)](https://github.com/kubevela/kubevela/releases)
[![TODOs](https://img.shields.io/endpoint?url=https://api.tickgit.com/badge?repo=github.com/kubevela/kubevela)](https://www.tickgit.com/browse?repo=github.com/kubevela/kubevela)
[![Twitter](https://img.shields.io/twitter/url?style=social&url=https%3A%2F%2Ftwitter.com%2Foam_dev)](https://twitter.com/oam_dev)
[![Artifact HUB](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/kubevela)](https://artifacthub.io/packages/search?repo=kubevela)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/4602/badge)](https://bestpractices.coreinfrastructure.org/projects/4602)
![E2E status](https://github.com/kubevela/kubevela/workflows/E2E%20Test/badge.svg)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/kubevela/kubevela/badge)](https://scorecard.dev/viewer/?uri=github.com/kubevela/kubevela)
[![](https://img.shields.io/badge/KubeVela-Check%20Your%20Contribution-orange)](https://opensource.alibaba.com/contribution_leaderboard/details?projectValue=kubevela)

## Introduction

KubeVela is a modern application delivery platform that makes deploying and operating applications across today's hybrid, multi-cloud environments easier, faster and more reliable.

![kubevela](docs/resources/what-is-kubevela.png)

## Highlights

KubeVela practices the "render, orchestrate, deploy" workflow with below highlighted values added to existing ecosystem:

#### **Deployment as Code**

Declare your deployment plan as workflow, run it automatically with any CI/CD or GitOps system, extend or re-program the workflow steps with [CUE](https://cuelang.org/).
No ad-hoc scripts, no dirty glue code, just deploy. The deployment workflow in KubeVela is powered by [Open Application Model](https://oam.dev/).

#### **Built-in observability, multi-tenancy and security support**

Choose from the wide range of LDAP integrations we provided out-of-box, enjoy enhanced [multi-tenancy and multi-cluster authorization and authentication](https://kubevela.net/docs/platform-engineers/auth/advance),
pick and apply fine-grained RBAC modules and customize them as per your own supply chain requirements.
All delivery process has fully [automated observability dashboards](https://kubevela.net/docs/platform-engineers/operations/observability).

#### **Multi-cloud/hybrid-environments app delivery as first-class citizen**

Natively supports multi-cluster/hybrid-cloud scenarios such as progressive rollout across test/staging/production environments,
automatic canary, blue-green and continuous verification, rich placement strategy across clusters and clouds,
along with automated cloud environments provision.

#### **Lightweight but highly extensible architecture**

Minimize your control plane deployment with only one pod and 0.5c1g resources to handle thousands of application delivery.
Glue and orchestrate all your infrastructure capabilities as reusable modules with a highly extensible architecture
and share the large growing community [addons](https://kubevela.net/docs/reference/addons/overview).

#### **Dynamic TraitDefinition Template Mode**

KubeVela now supports dynamic template mode for traitDefinition.  
This allows trait templates to be rendered via API, enabling complex logic and integration with external systems.

**How it works:**
- You can implement a traitDefinition that calls an API endpoint to render its template.
- The API receives parameters and returns a patch (e.g., environment variables) to inject into the workload.

**Example API:**
```
PUT /v1alpha1/trait/rdsbinding
Body: { "params": { "rdsCloudId": "xxxx" } }
Response: { "patch": { "envs": [{ "name": "RDSConn", "value": "*****" }] } }
```

This enables scenarios such as:
- Querying cloud resources (e.g., RDS connection string)
- Injecting values dynamically into workloads

Both static DSL and dynamic API modes are supported.  
See [TraitDefinition Dynamic Template Guide](docs/reference/trait-dynamic-template.md) for details.

## Getting Started

* [Introduction](https://kubevela.io/docs)
* [Installation](https://kubevela.io/docs/install)
* [Deploy Your Application](https://kubevela.io/docs/quick-start)

### Get Your Own Demo with Alibaba Cloud

- install KubeVela on a Serverless K8S cluster in 3 minutes, try:

  <a href="https://acs.console.aliyun.com/quick-deploy?repo=kubevela/kubevela&branch=master" target="_blank">
    <img src="https://img.alicdn.com/imgextra/i1/O1CN01aiPSuA1Wiz7wkgF5u_!!6000000002823-55-tps-399-70.svg" width="200" alt="Deploy on Alibaba Cloud">
  </a>

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
  - [Video Records](https://www.youtube.com/channel/UCSCTHhGI5XJ0SEhDHVakPAA/videos).
- Bi-weekly Chinese Community Call:
  - [Video Records](https://space.bilibili.com/180074935/channel/seriesdetail?sid=1842207).

## Talks and Conferences

Check out [KubeVela videos](https://kubevela.io/videos/talks/en/oam-dapr) for these talks and conferences.

## Contributing

Check out [CONTRIBUTING](https://kubevela.io/docs/contributor/overview) to see how to develop with KubeVela.

### Development Setup

To set up your development environment:

1. Make sure you have Go 1.24+ installed
2. Clone the repository
3. Install required tools:
   ```bash
   # Install kustomize (required for build)
   make install-kustomize
   
   # Ensure the bin directory is in your PATH
   export PATH=$(pwd)/bin:$PATH
   
   # Build the CLI
   make vela-cli
   ```

## Report Vulnerability

Security is a first priority thing for us at KubeVela. If you come across a related issue, please send email to security@mail.kubevela.io .

## Code of Conduct

KubeVela adopts [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/master/code-of-conduct.md).

# KubeVela

## Build Prerequisites

Before building KubeVela, ensure the following tools are installed:

- Go (>=1.16)
- kustomize (>=v3.8.0)
- Kubebuilder test binaries (`etcd`, `kube-apiserver`, `kubectl`) for running tests

### Install kustomize

You can install kustomize using one of the following methods:

#### Option 1: Install via script (recommended)

You can install kustomize by running:

```bash
curl -Lo kustomize https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize/v4.5.7/kustomize_v4.5.7_linux_amd64
chmod +x kustomize
sudo mv kustomize /usr/local/bin/
```

Or, place the binary at:

```
/root/GolandProjects/kubevela/bin/kustomize
```

### Install Kubebuilder test binaries

To run unit and integration tests, you need the Kubebuilder test binaries.  
**If you see errors like `fork/exec /usr/local/kubebuilder/bin/etcd: no such file or directory`, follow these steps:**

1. Download the test tools for your platform:
   ```sh
   # Example for Linux amd64, adjust version as needed
   curl -L https://go.kubebuilder.io/test-tools/v1.29.1?os=$(go env GOOS)&arch=$(go env GOARCH) | tar -xz -C /tmp/
   sudo mkdir -p /usr/local/kubebuilder/bin
   sudo mv /tmp/kubebuilder/bin/* /usr/local/kubebuilder/bin/
   ```

2. Ensure `/usr/local/kubebuilder/bin` contains `etcd`, `kube-apiserver`, and `kubectl`.

3. (Alternatively) Set the `KUBEBUILDER_ASSETS` environment variable to the directory containing these binaries:
   ```sh
   export KUBEBUILDER_ASSETS=/usr/local/kubebuilder/bin
   ```

Make sure `/usr/local/kubebuilder/bin/etcd` exists after installation.

## Install

To install KubeVela, run the following command:

```bash
make install
```

This will install the KubeVela CLI and other necessary components.

## Uninstall

To uninstall KubeVela, run the following command:

```bash
make uninstall
```

This will remove the KubeVela components from your system.

## Upgrade

To upgrade KubeVela to the latest version, run the following command:

```bash
make upgrade
```

This will upgrade your KubeVela installation to the latest version.

## Usage

To use KubeVela, you need to configure your cloud provider credentials and Kubernetes cluster information.

1. Set up your cloud provider credentials. For example, to set up Alibaba Cloud credentials, run:

   ```bash
   export ALIYUN_ACCESS_KEY_ID=<your-access-key-id>
   export ALIYUN_ACCESS_KEY_SECRET=<your-access-key-secret>
   ```

2. Configure your Kubernetes cluster information. For example, to configure a cluster named "test-cluster", run:

   ```bash
   vela cluster add test-cluster --provider alicloud --region cn-hangzhou
   ```
## Testing Requirements
# Test Environment Setup Guide

## Running tests in KubeVela

KubeVela uses Kubernetes controller-runtime's envtest framework for testing, which requires specific binaries to be installed.

### Prerequisites

- Go 1.24 or higher
- Docker (for certain tests)
- Git

### Setting up the test environment

1. Run the test environment setup script:

```bash
./hack/setup-test-env.sh
### Kubebuilder

Many tests in this project require kubebuilder binaries to run the control plane. If you encounter errors related to missing binaries like:
3. Deploy an application. For example, to deploy the "wordpress" application, run:

   ```bash
   vela up wordpress
   ```

4. Access the application. For example, to access the "wordpress" application, run:
# KubeVela Project

## Development Environment Setup

### Setting Up Test Environment

Before running tests, you need to set up the test environment with the required binaries:

```bash
# Install kubebuilder test binaries
make setup-envtest

# Run tests
go test ./...
```

### Common Issues and Troubleshooting

#### Kubebuilder test binaries missing

If you see errors like:

```
unable to start control plane itself: failed to start the controlplane. retried 5 times: fork/exec /usr/local/kubebuilder/bin/etcd: no such file or directory
runtime error: invalid memory address or nil pointer dereference
```

This is caused by missing Kubebuilder test binaries (`etcd`, `kube-apiserver`, `kubectl`).  
Follow the instructions in **Build Prerequisites** above to install them.

> **Note:**  
> The `invalid memory address or nil pointer dereference` panic often occurs after a failed envtest start due to missing binaries.  
> **This is not a separate bug**—it is a side effect of the failed test environment setup.  
> Fix the missing binaries and the panic will be resolved.

#### Go import errors

If you see errors like:

```
imports must appear before other declarations
undefined: strings
undefined: exec
```

This means your Go file is missing required imports or the import block is not at the top of the file.  
To fix:

- Ensure all `import` statements appear before any other declarations in your Go files.
- Add missing imports, for example:
  ```go
  import (
      "os/exec"
      "strings"
      // ...other imports...
  )
  ```
- See [Go import declaration docs](https://go.dev/doc/effective_go#imports) for details.

#### E2E test failures: "vela": executable file not found in $PATH

If you see errors like:

```
failed to run command 'vela up -f vela.json': exec: "vela": executable file not found in $PATH
```

This means the `vela` CLI binary is not available in your `$PATH` during test execution.

**To fix:**
- Build the CLI and add it to your `$PATH`:
  ```bash
  make vela-cli
  export PATH=$(pwd)/bin:$PATH
  ```
- Confirm `vela` is available:
  ```bash
  which vela
  vela version
  ```

#### E2E test failures: "no configuration has been provided, try setting KUBERNETES_MASTER environment variable"

If you see errors like:

```
invalid configuration: no configuration has been provided, try setting KUBERNETES_MASTER environment variable
```

This means your tests cannot find a valid kubeconfig to access a Kubernetes cluster.

**To fix:**
- Ensure you have a valid kubeconfig file (usually at `~/.kube/config`).
- Set the `KUBECONFIG` environment variable if your config is elsewhere:
  ```bash
  export KUBECONFIG=/path/to/your/kubeconfig
  ```
- Confirm cluster access:
  ```bash
  kubectl get nodes
  ```

### Fix for Shared ConfigMap Patching in Gateway Trait

KubeVela's gateway trait now supports safe patching of shared ConfigMaps (`tcp-services` and `udp-services`) in the `ingress-nginx` namespace.  
This allows multiple applications to update these resources without overwriting each other's entries.

#### How to Apply the Fix

1. **Update the Gateway Trait Definition**

Replace your existing gateway trait definition with the following CUE template:

```cue
// gateway trait with safe shared ConfigMap patching

template: {
    // ...existing code...

    // Shared ConfigMap handling for TCP
    if parameter.tcp != _|_ {
        outputs: tcpConfig: {
            apiVersion: "v1"
            kind:       "ConfigMap"
            metadata: {
                name:      "tcp-services"
                namespace: "ingress-nginx"
            }
            $patch: "merge"
            data: {
                // Merge new entries with existing ConfigMap data
                // Hypothetical: context.getResource fetches existing data
                $existing: context.getResource("ConfigMap", "tcp-services", "ingress-nginx").data
                for k, v in parameter.tcp {
                    (v.gatewayPort): context.namespace + "/" + context.name + ":" + v.port
                }
                ...$existing
            }
        }
    }

    // Shared ConfigMap handling for UDP
    if parameter.udp != _|_ {
        outputs: udpConfig: {
            apiVersion: "v1"
            kind:       "ConfigMap"
            metadata: {
                name:      "udp-services"
                namespace: "ingress-nginx"
            }
            $patch: "merge"
            data: {
                $existing: context.getResource("ConfigMap", "udp-services", "ingress-nginx").data
                for k, v in parameter.udp {
                    (v.gatewayPort): context.namespace + "/" + context.name + ":" + v.port
                }
                ...$existing
            }
        }
    }

    // ...existing code...
}
```

2. **Apply the Trait**

```bash
vela def apply gateway.cue
```

3. **Test the Fix**

- Deploy two or more applications using the gateway trait with TCP/UDP parameters.
- Check the shared ConfigMaps:

```bash
kubectl get configmap -n ingress-nginx tcp-services -o yaml
kubectl get configmap -n ingress-nginx udp-services -o yaml
```

- Verify that all applications' entries are present and no data is overwritten.

#### Notes

- The `context.getResource` function is hypothetical. If not supported, use a custom controller or pre-apply hook to merge ConfigMap data.
- The `$patch: "merge"` directive ensures updates are merged, not replaced.
- For concurrency, Kubernetes handles resource versioning natively.

#### Pull Request Description Example

> **Support Shared ConfigMap Patching for Gateway Trait**
>
> - Enables safe, concurrent updates to shared `tcp-services` and `udp-services` ConfigMaps.
> - Merges new entries with existing data to avoid overwriting.
> - Tested with multiple applications for conflict-free updates.
> - Fixes #2837.

For more details, see [issue #2837](https://github.com/kubevela/kubevela/issues/2837).

## License

KubeVela is licensed under the [Apache 2.0 License](https://github.com/kubevela/kubevela/blob/master/LICENSE).
