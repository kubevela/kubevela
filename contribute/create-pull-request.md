# Create a pull request

We're excited that you're considering making a contribution to the KubeVela project!
This document guides you through the process of creating a [pull request](https://help.github.com/en/articles/about-pull-requests/).

## Before you begin

We know you're excited to create your first pull request. Before we get started, read these resources first:

- Learn how to start [Contributing to KubeVela](/CONTRIBUTING.md).
- Make sure your code follows the relevant [style guides](/contribute/style-guides).

## Your first pull request

If this is your first time contributing to an open-source project on GitHub, make sure you read about [Creating a pull request](https://help.github.com/en/articles/creating-a-pull-request).

To increase the chance of having your pull request accepted, make sure your pull request follows these guidelines:

- Title and description matches the implementation.
- Commits within the pull request follow the [Formatting guidelines](#Formatting-guidelines).
- The pull request closes one related issue.
- The pull request contains necessary tests that verify the intended behavior.
- If your pull request has conflicts, rebase your branch onto the main branch.

If the pull request fixes a bug:

- The pull request description must include `Closes #<issue number>` or `Fixes #<issue number>`.
- To avoid regressions, the pull request should include tests that replicate the fixed bug.

Please refer to the [code conventions](/contribute/coding-conventions.md) for better code style and conventions.

## Code review

Once you've created a pull request, the next step is to have someone review your change.
A review is a learning opportunity for both the reviewer and the author of the pull request.

If you think a specific person needs to review your pull request, then you can tag them in the description or in a comment.
Tag a user by typing the `@` symbol followed by their GitHub username.

We recommend that you read [How to do a code review](https://google.github.io/eng-practices/review/reviewer/) to learn more about code reviews.

## Formatting guidelines

A well-written pull request minimizes the time to get your change accepted.
These guidelines help you write good commit messages and descriptions for your pull requests.

### Commit message format

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

#### Area

The area should use upper camel case, e.g. UpperCamelCase.

Prefer using one of the following areas:

- **Application:** Changes to the application controller.
- **Component:** Changes to the component related code or definition controller.
- **Trait:** Changes to the trait related code or definition controller.
- **CUE:** Changes to the CUE related logic.
- **Docs:** Changes to documentation.
- **AppConfig:** Changes to AppConfig related code.

**Examples**

- `Application: Support workflow in application controller`
- `CUE: Fix patch parse issues`
- `Docs: Changed url to URL in all documentation files`

### Pull request titles

The KubeVela team _squashes_ all commits into one when we accept a pull request.
The title of the pull request becomes the subject line of the squashed commit message.
We still encourage contributors to write informative commit messages, as they become a part of the Git commit body.

We use the pull request title when we generate change logs for releases. As such, we strive to make the title as informative as possible.

Make sure that the title for your pull request uses the same format as the subject line in the commit message. If the format is not followed, we will add a label `title-needs-formatting` on the pull request.

### Pass all the CI checks

Before merge, All test CI should pass green.
The `codecov/project` should also pass. This means the coverage should not drop. See [Codecov commit status](https://docs.codecov.io/docs/commit-status#project-status).
