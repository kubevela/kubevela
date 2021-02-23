# KubeVela Dashboard

## Quick start

In the root folder of this project, run `make start-dashboard` to start backend OpenAPI server and Dashboard at the same time.

```shell
➜  xxx/src/github.com/oam-dev/kubevela $ make start-dashboard
go run references/cmd/apiserver/main.go &
cd dashboard && npm install && npm start && cd ..
I0205 11:25:55.742786    5535 request.go:621] Throttling request took 1.002149891s, request: GET:https://47.242.145.141:6443/apis/coordination.k8s.io/v1beta1?timeout=32s
[GIN-debug] [WARNING] Running in "debug" mode. Switch to "release" mode in production.
 - using env:	export GIN_MODE=release
 - using code:	gin.SetMode(gin.ReleaseMode)

[GIN-debug] POST   /api/envs/                --> github.com/oam-dev/kubevela/references/apiserver.(*APIServer).CreateEnv-fm (6 handlers)
[GIN-debug] PUT    /api/envs/:envName        --> github.com/oam-dev/kubevela/references/apiserver.(*APIServer).UpdateEnv-fm (6 handlers)
[GIN-debug] GET    /api/envs/:envName        --> github.com/oam-dev/kubevela/references/apiserver.(*APIServer).GetEnv-fm (6 handlers)
[GIN-debug] GET    /api/envs/                --> github.com/oam-dev/kubevela/references/apiserver.(*APIServer).ListEnv-fm (6 handlers)

> fsevents@1.2.13 install /Users/zhouzhengxi/Programming/golang/src/github.com/oam-dev/kubevela/dashboard/node_modules/watchpack-chokidar2/node_modules/fsevents
> node install.js

  SOLINK_MODULE(target) Release/.node
  CXX(target) Release/obj.target/fse/fsevents.o
  SOLINK_MODULE(target) Release/fse.node

> ejs@2.7.4 postinstall /Users/zhouzhengxi/Programming/golang/src/github.com/oam-dev/kubevela/dashboard/node_modules/umi-webpack-bundle-analyzer/node_modules/ejs
> node ./postinstall.js

Thank you for installing EJS: built with the Jake JavaScript build tool (https://jakejs.com/)


> kubevela@0.0.1 postinstall /Users/zhouzhengxi/Programming/golang/src/github.com/oam-dev/kubevela/dashboard
> umi g tmp

added 1234 packages from 743 contributors, removed 49 packages, updated 85 packages and audited 3208 packages in 41.551s

235 packages are looking for funding
  run `npm fund` for details

found 19 vulnerabilities (18 low, 1 high)
  run `npm audit fix` to fix them, or `npm audit` for details

> kubevela@0.0.1 start /Users/zhouzhengxi/Programming/golang/src/github.com/oam-dev/kubevela/dashboard
> umi dev

Starting the development server...

✔ Webpack
  Compiled successfully in 34.81s

 DONE  Compiled successfully in 34815ms                                                                                                                                                                                                            11:27:12 AM


  App running at:
  - Local:   http://localhost:8002 (copied to clipboard)
  - Network: http://30.240.99.101:8002
```

## Development

### Install dependencies

```bash
npm install
```

### Start up

```bash
npm start
```
