---
title:  Worker
---

## Description

Describes long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to receive external network traffic.

## Samples

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-worker
spec:
  components:
    - name: myworker
      type: worker
      properties:
        image: "busybox"
        cmd:
          - sleep
          - "1000"
```

## Specification

```console
# Properties
+-------+----------------------------------------------------+----------+----------+---------+
| NAME  |                    DESCRIPTION                     |   TYPE   | REQUIRED | DEFAULT |
+-------+----------------------------------------------------+----------+----------+---------+
| cmd   | Commands to run in the container                   | []string | false    |         |
| image | Which image would you like to use for your service | string   | true     |         |
+-------+----------------------------------------------------+----------+----------+---------+
``` 
