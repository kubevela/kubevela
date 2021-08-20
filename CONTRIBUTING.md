# CONTRIBUTING Guide

## About KubeVela

KubeVela project is initialized and maintained by the cloud native community since day 0 with [bootstrapping contributors from 8+ different organizations](https://github.com/oam-dev/kubevela/graphs/contributors).
We intend for KubeVela to have an open governance since the very beginning and donate the project to neutral foundation as soon as it's released.
To help us create a safe and positive community experience for all, we require all participants to adhere to the [Code of Conduct](./CODE_OF_CONDUCT.md).

This document is a guide to help you through the process of contributing to KubeVela.

## Become a contributor

You can contribute to KubeVela in several ways. Here are some examples:

* Contribute to the KubeVela codebase.
* Contribute to the [KubeVela docs](https://github.com/oam-dev/kubevela.io).
* Report and triage bugs.
* Develop community CRD operators as workload or trait and contribute to [catalog](https://github.com/oam-dev/catalog).
* Write technical documentation and blog posts, for users and contributors.
* Organize meetups and user groups in your local area.
* Help others by answering questions about KubeVela.

For more ways to contribute, check out the [Open Source Guides](https://opensource.guide/how-to-contribute/).

### Commit Conventions

KubeVela follows the [conventional-commits](https://www.conventionalcommits.org/en/v1.0.0/) and [commit messages best practices](https://chris.beams.io/posts/git-commit/) to improve better history information.

The commit message should be structured as follows:

```
<type>[optional scope]: <subject>

[optional body]
```

#### Examples:

Commit message with scope:

```
Feat(lang): add polish language
```

Commit message with no body:

```
Docs: correct spelling of CHANGELOG
```

Commit message with multi-paragraph body:

```
Fix: correct minor typos in code

see the issue for details

on typos fixed.

Reviewed-by: Z
Refs #133
```

#### `<type>` (required)

Type is required to better capture the area of the commit, based on the [Angular convention](https://github.com/angular/angular/blob/22b96b9/CONTRIBUTING.md#-commit-message-guidelines).

We capitalize the `<type>` to make sure the subject line is capitalized. `<type>` can be one of the following:

* **Feat**: A new feature
* **Fix**: A bug fix
* **Docs**: Documentation only changes
* **Build**: Changes that affect the build system or external dependencies 
* **Style**: Changes that do not affect the meaning of the code (white-space, formatting, missing semi-colons, etc)
* **Refactor**: A code change that neither fixes a bug nor adds a feature
* **Perf**: A code change that improves performance
* **Test**: Adding missing or correcting existing tests
* **Chore**: Changes to the build process or auxiliary tools and libraries such as documentation generation

#### `<scope>` (optional)

Scope is optional, it may be provided to a commit’s type, to provide additional contextual information and is contained within parenthesis, it is could be anything specifying place of the commit change. Github issue link is
also a valid scope. For example: Fix(cli), Feat(api), Fix(#233), etc.

You can use `*` when the change affects more than a single scope.

#### `<subject>` (required)

The subject MUST immediately follow the colon and space after the type/scope prefix. The description is a short summary of the code changes, e.g., "Fix: array parsing issue when multiple spaces were contained in string", instead of "Fix: bug".

#### `<body>` (optional)

A longer commit body may be provided after the short subject, providing additional contextual information about the code changes. The body MUST begin one blank line after the description.

### Report bugs

Before submitting a new issue, try to make sure someone hasn't already reported the problem.
Look through the [existing issues](https://github.com/oam-dev/kubevela/issues) for similar issues.

Report a bug by submitting a [bug report](https://github.com/oam-dev/kubevela/issues/new?assignees=&labels=kind%2Fbug&template=bug_report.md&title=).
Make sure that you provide as much information as possible on how to reproduce the bug.

Follow the issue template and add additional information that will help us replicate the problem.

#### Security issues

If you believe you've found a security vulnerability, please read our [security policy](https://github.com/oam-dev/kubevela/blob/master/SECURITY.md) for more details.

### Suggest enhancements

If you have an idea to improve KubeVela, submit an [feature request](https://github.com/oam-dev/kubevela/issues/new?assignees=&labels=kind%2Ffeature&template=feature_request.md&title=%5BFeature%5D).

### Triage issues

If you don't have the knowledge or time to code, consider helping with _issue triage_. The community will thank you for saving them time by spending some of yours.

Read more about the ways you can [Triage issues](/contribute/triage-issues.md).

### Answering questions

If you have a question and you can't find the answer in the [documentation](https://kubevela.io/docs/),
the next step is to ask it on the [github discussion](https://github.com/oam-dev/kubevela/discussions).

It's important to us to help these users, and we'd love your help. You can help other KubeVela users by answering [their questions](https://github.com/oam-dev/kubevela/discussions).

### Your first contribution

Unsure where to begin contributing to KubeVela? Start by browsing issues labeled `good first issue` or `help wanted`.

- [Good first issue](https://github.com/oam-dev/kubevela/labels/good%20first%20issue) issues are generally straightforward to complete.
- [Help wanted](https://github.com/oam-dev/kubevela/labels/help%20wanted) issues are problems we would like the community to help us with regardless of complexity.

If you're looking to make a code change, see how to set up your environment for [local development](contribute/developer-guide.md).

When you're ready to contribute, it's time to [Create a pull request](/contribute/create-pull-request.md).
