# Contributing to KubeVela

Thanks for your interest in KubeVela. Contributions of every kind are welcome, whether
you are fixing a typo, triaging an issue, improving the docs, or shipping a new feature.
This page is a map: it covers what you need before your first pull request and links out
to the full guide for everything else.

## Code of Conduct

KubeVela follows the CNCF Code of Conduct. By participating, you agree to uphold it.
See [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md).

## Ways to contribute

- **Code**
  - Pick up a [good first issue](https://github.com/kubevela/kubevela/labels/good%20first%20issue).
  - Follow the [code conventions](https://kubevela.io/docs/contributor/code-conventions) and [test principles](https://kubevela.io/docs/contributor/principle-of-test).
  - See the [code contribution guide](https://kubevela.io/docs/contributor/code-contribute) for local setup and the pull request process.
- **Docs**
  - The docs live in [kubevela/kubevela.io](https://github.com/kubevela/kubevela.io).
  - See its [developer guide](https://github.com/kubevela/kubevela.io/blob/main/README.md) for how to write and preview them.
- **Issue triage**
  - Help reproduce, label, and narrow down reported bugs.
  - See [ISSUE_TRIAGE.md](https://github.com/kubevela/community/blob/main/ISSUE_TRIAGE.md).
- **Answer questions**: help other users in [GitHub Discussions](https://github.com/kubevela/kubevela/discussions) and on Slack.
- **Everything else**
  - Blog posts, talks, case studies, and translations all count.
  - See the [non-code contribution guide](https://kubevela.io/docs/contributor/non-code-contribute).

## Quick start

Build the CLI, run the unit tests, and lint before opening a pull request:

```bash
make             # build the vela CLI to bin/vela
make test        # run unit tests
make reviewable  # lint, generate, and get the PR ready for review
```

These assume the prerequisites (Go, kustomize, CUE, and friends) are already
installed. See the [code contribution guide](https://kubevela.io/docs/contributor/code-contribute)
for installing those, running `vela-core` and VelaUX locally, and end-to-end tests.

## Commit messages

Commits follow `<Type>[optional scope]: <subject>`, with the type capitalized:

```
Fix: correct minor typos in code
Feat(cli): add polish language
Docs: changed url to URL in all documentation files
```

`<Type>` is one of Feat, Fix, Docs, Build, Style, Refactor, Perf, Test, or
Chore. Pull requests are squash-merged, so the PR title becomes the commit
message, use the same format there too. See
[Formatting guidelines](https://kubevela.io/docs/contributor/code-contribute#formatting-guidelines)
for the full convention, including scopes and areas.

## Sign your commits (DCO)

Every commit must carry a `Signed-off-by` line certifying you wrote the code and can
contribute it under the project license. Add it with `-s`:

```bash
git commit -s -m "your commit message"
```

Without it, your pull request may not be able to merge. For the full explanation and
how to fix commits you already pushed, see
[Sign Your Commits (DCO)](https://kubevela.io/docs/contributor/code-contribute#sign-your-commits-dco).

## More

- **Full Contributor Guide**: [kubevela.io/docs/contributor/overview](https://kubevela.io/docs/contributor/overview), covering development setup, testing, and the pull request process.
- **Governance**: [GOVERNANCE.md](./GOVERNANCE.md) explains roles, responsibilities, and how decisions get made.
- **Security**: do not open a public issue for a vulnerability. Follow [SECURITY.md](./SECURITY.md).
- **Communication**: Slack, community meetings, and how to get involved are covered in [COMMUNITY.md](./COMMUNITY.md).
