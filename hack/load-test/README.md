# Load Test Guide

## Files

Two application templates are provided in [app-templates](./app-templates) folder. The [community template](app-templates/community.yaml) is a standard template which can be applied in a standard KubeVela environment. The [SAE template](app-templates/sae.yaml), on the other hand, requires support for extended definitions (env & cloneset-service) and OpenKruise to be installed. The `tolerate-hollow-node` trait is used to add toleration to pods in load test applications, which makes them allocatable for hollow nodes mocked by `kubemark`.

The `dashboard.json` is a grafana dashboard file that can track several groups of key metrics when running load tests.

## Steps

1. **Prepare Kubernetes Clusters.** The load test is conducted under ACK (Alibaba Cloud Container Service for Kubernetes) environment. You can set up your own Kubernetes cluster environment if you would like to run your own pressure test. 
2. **Prepare Kubernetes Environments.** You need to install KubeVela first in your environment and add program flag `--perf-enabled` to the pods of KubeVela to enable performance monitoring. Remember to disable MutatingWebhookConfigurations and ValidatingWebhookConfigurations if you only care about the KubeVela controller performance.
3. **[Optional] Prepare Kubemark Environments.** The `tolerate-hollow-node` trait need to be installed if you would like to make load test on hollow nodes mocked by `kubemark`. Another issue for using `kubemark` is that you need to ensure pods created by pressure test applications would be assigned to hollow nodes while other pods like KubeVela controller must not be assigned on these hollow nodes. You can achieve that by adding taints to nodes and adding nodeSelector or toleration to pods.  
4. **Prepare Monitoring Tools.** Install Grafana, Loki, Prometheus in your Kubernetes which can help you watch load test progress and discover problems. If you have `vela` command line tool installed, you can run `vela addon enable observability` to enable it.
5. **Run Pressure Test.** Start load test by rendering application templates with different IDs to generate application instances and apply them to Kubernetes at a desired creation speed. Wait for a while (could be hours) and delete them. This is standard progress of the pressure test. More mutating actions could be injected.
6. **Check Result.** You can upload the grafana dashboard file to the Grafana website exposed from your cluster. Then you can check the result of the load test.

## Use of application bulk deploy scripts

### Setup

Run `SHARD=3 CLUSTER=2 bash bootstrap.sh`. This will create 3 namespaces `load-test-0`, `load-test-1`, `load-test-2` to local cluster and all managed clusters.

### Deploy Apps

#### Basic

Run `BEGIN=0 SIZE=1000 SHARD=3 WORKER=6 bash deploy.sh` to deploy 1000 application (id from 0 to 1000) to 3 shard in 6 parallel threads.

#### Version Update

By default, the deployed apps will use variable `VERSION=1`. You can set this variable to change the version of applications and test version upgrades.

#### Choose different app templates

Set `TEMPLATE=heavy` will use the `app-templates/heavy.yaml` as the application template to deploy.

#### Multicluster Apps

Set `CLUSTER=3` will inject the `CLUSTER` variable to the app template. You can use `TEMPLATE=multicluster` or `TEMPLATE=region` to make multicluster application delivery.

> To make multicluster load testing environment, you can set up multiple k3d instances and register them in the control plane.

#### QPS

By default, there is no rate limit for the client. If you want to set the QPS for each worker, you can use `QPS=2`.

### Cleanup

Run `SHARD=3 WORKER=4 BEGIN=0 SIZE=1000 bash cleanup.sh` to delete `app-0` to `app-999` from namespace `load-test-0` to `load-test-2` in 4 threads.
