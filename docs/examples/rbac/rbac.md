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
permissions: ["app-manage"]
```

```yaml
name: admin
permissions: ["all"]
```

Permission:

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
name: app1-manage
project: demo
resource: ["project:demo/application:app1/*"]
actions: ["*"]
effect: Allow
principal: {}
condition: {}

name: app2-manage
project: demo
resource: ["project:demo/application:app2/*"]
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

PermissionTemplate:

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
