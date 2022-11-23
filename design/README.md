# KubeVela Enhancement Proposals (KEPs)

This directory is a place to propose and discuss new ideas of KubeVela concepts, designs, architectures and techniques.

### When do we need KEPs

When major changes are intended to be made to KubeVela project, we need KEPs. Major changes includes:
- New project-level features that add modules to the architecture, like new Controller or APIServer.
- Break changes to the core concepts of KubeVela, such as Application, Workflow, Component, etc.
- Techniques or domains that lots of related enhancements need to be added to KubeVela, like multi-cluster, observability, etc.

Changes to the internal mechanism of core KubeVela are recommended to add proposals as well, including:
- Application behaviours and related policies: State-keep, garbage-collect, resource dispatch, etc.
- API changes of auxiliary resources in KubeVela, such as ApplicationRevision, ResourceTracker.
- New concepts and layers in KubeVela APIServer on VelaUX, such as Project, Target, etc.

Minor changes and enhancements do not necessarily need to be included, but instead recommended to be tracked by issues, such as 
- New addons.
- New Component/Trait/WorkflowStep definitions.
- New additional function APIs in APIServer.
- Bug detection and fixes.
- Auxiliary commands in CLI.

### Areas

There are several directories contained. Each directory contains the KEPs in specific area.

- **/vela-core**: The proposals of features and changes to the core KubeVela, including Application APIs, internal mechanisms, auxiliary policies, etc.
- **/vela-cli**: The proposals of features to the KubeVela CLI, such as `vela top`, `vela def`.
- **/api**: The proposals of the interfaces KubeVela exposes to users, such as command line args for the core controller.
- **/platform**: The proposals of integrating features in various related areas outside KubeVela, such as edge computing, artificial intelligence.
- **/resources**: The related images embedded in the design documentations.

### Writing a new Proposal

The aim of a proposal is to communicate designs with others and give KubeVela users some basic ideas of how features and evolved and developed.

To reach that, there are several things seed to be included in a proposal.
1. The background of the feature or change, which explains why we need it.
2. The goals and non-goals for the proposal.
3. The potential technical solutions for the proposal and comparisons between various solutions. (Single solution is also acceptable.)
4. How we should move on for the proposal. The estimated milestones or timelines for the feature development.

### Submitting a new proposal

We recommend to use the [template](./TEMPLATE.md) to start a new proposal.
After finishing the proposal in the proper directory, raise a pull request to add the proposal to the main repo.
If there are any issues related to the proposal, you can also add links to the issues in the pull request.