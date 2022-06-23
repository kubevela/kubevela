# Steps with if

Every step can specify a `if`, you can use the `if` to determine whether the step should be executed or not.

## Always

If you want to execute the step no matter what, for example, send a notification after the component is deployed even it's failed, you can use the `if` with the value `always` like:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: if-always-with-err
  namespace: default
spec:
  components:
  - name: err-component
    type: k8s-objects
    properties:
      objects:
        - err: "error case"
  workflow:
    steps:
    - name: apply-err-comp
      type: apply-component
      properties:
        component: err-component
    - name: notification
      type: notification
      if: always
      properties:
        slack:
          url:
            value: <your slack webhook url>
          message:
            text: always
```

## Custom Judgement

You can also write your own judgement logic to determine whether the step should be executed or not, note that the values of `if` will be executed as cue code. We support some built-in variables to use in `if`, they are:

* `status`: in this value, you can get the status of the step for judgement like `status.<step-name>.phase == "succeeded"`, or you can use the simplify way `status.<step-name>.succeeded`.
* `inputs`: in this value, you can get the inputs of the step for judgement like `inputs.<input-name> == "value"`.

### Status Example

If you want to control the step by the status of another step, you can follow the example:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: if-timeout
  namespace: default
spec:
  components:
  - name: comp-custom-timeout
    type: webservice
    properties:
      image: crccheck/hello-world
      port: 8000
  workflow:
    steps:
    - name: suspend
      timeout: 5s
      type: suspend
    - name: suspend2
      # or `status.suspend.reason == "Timeout"`
      if: status.suspend.timeout
      type: suspend
      timeout: 5s
    - name: notification-1
      type: notification
      if: suspend.timeout
      properties:
        slack:
          url:
            value: <your slack webhook url>
          message:
            text: suspend is timeout
    - name: notification-2
      type: notification
      if: status["notification-1"].succeeded
      properties:
        slack:
          url:
            value: <your slack webhook url>
          message:
            text: notification-1 is succeeded
```

### Inputs example

If you want to control the step by the inputs of another step, you can follow the example:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: if-input
  namespace: default
spec:
  components:
  - name: comp-custom-timeout
    type: webservice
    properties:
      image: crccheck/hello-world
      port: 8000
  workflow:
    steps:
    - name: suspend
      type: suspend
      timeout: 5s
      outputs:
        - name: test
          valueFrom: context.name + " message"
    - name: notification
      type: notification
      inputs:
        - from: test
          parameterKey: slack.message.text
      if: inputs.test == "if-input message"
      properties:
        slack:
          url:
            value: <your slack webhook url>
          message:
            text: from input
```