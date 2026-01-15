Schedule workflow restarts to enable periodic tasks, delayed execution, or time-based orchestration. The step uses exactly one of three timing modes: `at` for a specific timestamp, `after` for a relative delay, or `every` for recurring intervals.

```yaml
# Example 1: Fixed timestamp - restart at specific time
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: scheduled-app
  namespace: default
spec:
  components:
    - name: my-component
      type: webservice
      properties:
        image: nginx:latest
        port: 80
  workflow:
    steps:
      - name: deploy
        type: apply-component
        properties:
          component: my-component
      - name: schedule-restart
        type: restart-workflow
        properties:
          at: "2025-01-20T15:00:00Z"
---
# Example 2: Relative delay - restart after duration
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: delayed-restart-app
  namespace: default
spec:
  components:
    - name: batch-processor
      type: webservice
      properties:
        image: myapp/batch-processor:v1
        port: 8080
  workflow:
    steps:
      - name: deploy
        type: apply-component
        properties:
          component: batch-processor
      - name: schedule-restart-after
        type: restart-workflow
        properties:
          after: "1h"
---
# Example 3: Recurring - restart every interval
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: periodic-sync-app
  namespace: default
spec:
  components:
    - name: data-sync
      type: webservice
      properties:
        image: myapp/data-sync:v1
        port: 8080
  workflow:
    steps:
      - name: deploy
        type: apply-component
        properties:
          component: data-sync
      - name: schedule-recurring-restart
        type: restart-workflow
        properties:
          every: "24h"
```

**Use cases:**

- **Periodic tasks**: Schedule recurring workflow execution for data synchronization, batch processing, or scheduled maintenance
- **Delayed deployment**: Add a delay after initial deployment before triggering workflow restart
- **Time-based orchestration**: Coordinate workflows to run at specific times across multiple applications
