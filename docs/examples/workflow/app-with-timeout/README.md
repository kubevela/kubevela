# Timeout steps

Every step can specify a `timeout`, if the timeout expires and the step has not succeeded, the step will fail with the reason `Timeout`.

Here is an example:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-with-timeout
  namespace: default
spec:
  components:
  - name: comp
    type: webservice
    properties:
      image: crccheck/hello-world
      port: 8000
    traits:
    - type: scaler
      properties:
        replicas: 10
  workflow:
    steps:
    - name: apply
      timeout: 1m
      type: apply-component
      properties:
        component: comp
    - name: suspend
      type: suspend
      timeout: 5s
```

If the first step is succeeded in the time of `1m`, the second step will be executed. If the second step is not resumed in the time of `5s`, the suspend step will be failed with the reason `Timeout`, and the application will end up with the status of `WorkflowTerminated` like:

```yaml
status:
  status: workflowTerminated
  workflow:
    ...
    finished: true
    message: Terminated
    mode: StepByStep
    steps:
    - firstExecuteTime: "2022-06-22T09:19:42Z"
      id: gdcwh929ih
      lastExecuteTime: "2022-06-22T09:20:08Z"
      name: apply
      phase: succeeded
      type: apply-component
    - firstExecuteTime: "2022-06-22T09:20:08Z"
      id: rloz8axnju
      lastExecuteTime: "2022-06-22T09:20:13Z"
      name: suspend
      phase: failed
      reason: Timeout
      type: suspend
    suspend: false
    terminated: true
```