```yaml
# The Job runs in the Application's namespace as this service account, with only the rights the
# command uses: here, listing Applications. Never give it the controller's account.
apiVersion: v1
kind: ServiceAccount
metadata:
  name: vela-cli-runner
  namespace: vela-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: vela-cli-runner
  namespace: vela-system
rules:
  - apiGroups: ["core.oam.dev"]
    resources: ["applications"]
    verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: vela-cli-runner
  namespace: vela-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: vela-cli-runner
subjects:
  - kind: ServiceAccount
    name: vela-cli-runner
    namespace: vela-system
---
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: vela-cli
  namespace: vela-system
spec:
  components: []
  workflow:
    steps:
    - name: list-app
      type: vela-cli
      properties:
        serviceAccountName: vela-cli-runner
        command:
          - vela
          - ls
          - -n
          - vela-system
```
