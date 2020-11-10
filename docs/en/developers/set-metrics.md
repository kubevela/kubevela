# Monitoring Application

If your application has exposed metrics, you can easily setup monitoring system
with the help of `metric` capability.

Let's run [`christianhxc/gorandom:1.0`](https://github.com/christianhxc/prometheus-tutorial) as an example app.
The app will emit random latencies as metrics.

```bash
$ vela svc deploy metricapp -t webservice --image christianhxc/gorandom:1.0 --port 8080
```

Then add metric by:

```bash
$ vela metric metricapp
Adding metric for app metricapp
⠋ Deploying ...
✅ Application Deployed Successfully!
  - Name: metricapp
    Type: webservice
    HEALTHY Ready: 1/1
    Routes:
      - ✅ metric: Monitoring port: 8080, path: /metrics, format: prometheus, schema: http.
    Last Deployment:
      Created at: 2020-11-02 14:31:56 +0800 CST
      Updated at: 2020-11-02T14:32:00+08:00
```

The metrics trait will automatically discover port and label to monitor if no parameters specified.
If more than one ports found, it will choose the first one by default.


## Verify that the metrics are collected on prometheus

```shell script
kubectl --namespace monitoring port-forward `k -n monitoring get pods -l prometheus=oam -o name` 9090
```

Then access the prometheus dashboard via http://localhost:9090/targets
