---
title:  Configuring data/env in Application
---

`vela` provides a `config` command to manage config data.

## `vela config set`

```bash
$ vela config set test a=b c=d
reading existing config data and merging with user input
config data saved successfully ✅
```

## `vela config get`

```bash
$ vela config get test
Data:
  a: b
  c: d
```

## `vela config del`

```bash
$ vela config del test
config (test) deleted successfully
```

## `vela config ls`

```bash
$ vela config set test a=b
$ vela config set test2 c=d
$ vela config ls
NAME
test
test2
```

## Configure env in application

The config data can be set as the env in applications.

```bash
$ vela config set demo DEMO_HELLO=helloworld
```

Save the following to `vela.yaml` in current directory:

```yaml
name: testapp
services:
  env-config-demo:
    image: heroku/nodejs-hello-world
    config: demo
```

Then run:
```bash
$ vela up
Parsing vela.yaml ...
Loading templates ...

Rendering configs for service (env-config-demo)...
Writing deploy config to (.vela/deploy.yaml)

Applying deploy configs ...
Checking if app has been deployed...
App has not been deployed, creating a new deployment...
✅ App has been deployed 🚀🚀🚀
    Port forward: vela port-forward testapp
             SSH: vela exec testapp
         Logging: vela logs testapp
      App status: vela status testapp
  Service status: vela status testapp --svc env-config-demo
```

Check env var:

```
$ vela exec testapp -- printenv | grep DEMO_HELLO
DEMO_HELLO=helloworld
```
