---
title: 在应用程序中配置数据或环境
---

`vela` 提供 `config` 命令用于管理配置数据。

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

## 在应用程序中配置环境变量

可以在应用程序中将配置数据设置为环境变量。

```bash
$ vela config set demo DEMO_HELLO=helloworld
```

将以下内容保存为 `vela.yaml` 到当前目录中： 

```yaml
name: testapp
services:
  env-config-demo:
    image: heroku/nodejs-hello-world
    config: demo
```

然后运行：
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

检查环境变量：

```
$ vela exec testapp -- printenv | grep DEMO_HELLO
DEMO_HELLO=helloworld
```
