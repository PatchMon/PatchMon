# Docker builds for fork PRs

PRs from forks do not trigger Docker builds by default (for security).

To request a build and push to Harbor for your fork PR:

1. Open your PR from the fork.
2. Ask a maintainer to add the **`approved-for-build`** label.
3. Once the label is added, the workflow runs and pushes an image to `cr.dev.patchmon.cloud/pr/patchmon-server:pr-<number>`.
4. A comment with the image URL will be posted on the PR.

Maintainers: only add the label after reviewing the PR. The build will run the PR's code with access to the dev container registry server.
