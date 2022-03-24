# RBAC

User:

```yaml
name: user
userRoles: ["app-developer"]
...
```

ProjectUser:

```yaml
username: user
project: demo
userRoles: ["app-developer"]
```

Role:

```yaml
name: app-developer
project: demo
permPolicies: ["app-manage"]
```

```yaml
name: admin
permPolicies: ["all"]
```

PermPolicy:

```yaml
name: app-manage
project: demo
resource: ["project:demo/application:*"]
actions: ["*"]
effect: Allow
principal: {}
condition: {}
```

```yaml
name: cluster-manage
resource: ["cluster:*"]
actions: ["*"]
effect: Allow
principal: {}
condition: {}
```

```yaml
name: cluster-beijing-manage
resource: ["cluster:beijing"]
actions: ["*"]
effect: Allow
principal: {}
condition: {}
```

```yaml
name: all
resource: ["*"]
actions: ["*"]
effect: Allow
principal: {}
condition: {}
```

PermPolicyTemplate:

```yaml
name: app-manage
resource: ["project:${projectName}/application:*"]
actions: ["*"]
level: project
effect: Allow
principal: {}
condition: {}
```

```yaml
name: deny-delete-cluster
resource: ["cluster:*"]
actions: ["delete"]
level: platform
effect: Deny
```
