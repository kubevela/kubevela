# Github Utilities

## How to add a custom runner

1. Create an ECS that can connect github. Create a normal user(DON'T use `root`).
2. Install Dependencies:
  - Kind
  - Docker `apt install docker.io`, and the user to docker group(`usermod -aG docker <user>`)
  - Kubectl
  - Go(1.14 for now, must align with CI)
  - Helm v3
  - ginkgo
  - Add all these dependencies to $PATH.
3. Install Custom runner agent https://docs.github.com/en/actions/hosting-your-own-runners/adding-self-hosted-runners
4. Run the runner as service https://docs.github.com/en/actions/hosting-your-own-runners/configuring-the-self-hosted-runner-application-as-a-service