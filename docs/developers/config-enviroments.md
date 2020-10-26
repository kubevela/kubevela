# Configuring Deployment Environments

Before working with your application, you need to prepare a deployment environment (e.g. test, staging, prod etc) which will configure the workspace, email for certificate issuer and domain for your application.

## Create environment

> TODO `--namespace` and `--domain` should be able to skipped
> TODO why don't use xip.io as demo?

```console
$ vela env init demo --namespace demo --email my@email.com --domain kubevela.demo
ENVIROMENT demo CREATED, Namespace: demo, Email: my@email.com.
```

## Check the deployment environment metadata

```console
$ cat ~/.vela/envs/demo/config.json
  {"name":"demo","namespace":"demo","email":"my@email.com","domain":"kubevela.demo","issuer":"oam-env-demo"}
```