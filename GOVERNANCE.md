# Governance

This is a short map. The full governance document, including how decisions get
made and how it can itself be amended, lives in
[kubevela/community/GOVERNANCE.md](https://github.com/kubevela/community/blob/main/GOVERNANCE.md).

## Code of Conduct

KubeVela follows the CNCF Code of Conduct. See [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md).

## Meetings

Weekly community meetings are open to the public and are the primary place to
discuss pull requests before they merge. See
[community meetings](https://github.com/kubevela/community#community-meetings)
for the schedule and joining links.

## KubeVela Enhancement Proposals

Changes that introduce new behavior or modify existing requirements go through
a KubeVela Enhancement Proposal, tracked under
[design/](https://github.com/kubevela/kubevela/tree/master/design).

All changes, including simple additions and bug fixes, must have an associated
issue assigned by a maintainer before a pull request is raised. A pull request
without an assigned issue will not be picked up and may be closed. Automated
dependency and backport pull requests are exempt, and maintainers can raise
pull requests without an issue if needed. Security vulnerabilities must not be
reported in a public issue, follow [SECURITY.md](./SECURITY.md) instead. See
[CONTRIBUTING.md](./CONTRIBUTING.md).

## Membership

KubeVela has three levels of membership beyond contributor: Reviewer,
Approver, and Maintainer, each with more responsibility than the last. See
[community membership](https://github.com/kubevela/community/blob/main/community-membership.md)
for what each level requires, and
[OWNERS.md](https://github.com/kubevela/community/blob/main/OWNERS.md) for who
currently holds which role.

Maintainers who are unresponsive for more than six months are removed unless
the other maintainers agree by supermajority to extend that period. New
maintainers are added the same way, by a supermajority vote.

## Security

Security vulnerabilities are handled privately per [SECURITY.md](./SECURITY.md),
never as a public issue.

## Releases

KubeVela aims for a major or minor release at least once every two months,
with patch releases as needed. Only maintainers can cut a release. See
[Release Process And Cadence](https://kubevela.io/docs/contributor/release-process)
for the full cadence and branch cut process.
