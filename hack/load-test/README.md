# Pressure Test Guide

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