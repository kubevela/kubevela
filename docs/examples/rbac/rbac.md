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
resource: Project/xxx/Application/*
actions: ["*"]
effect: Allow
principal: {}
condition: {}
```

```yaml
name: cluster-manage
resource: Cluster/*
actions: ["*"]
effect: Allow
principal: {}
condition: {}
```

```yaml
name: cluster-beijing-manage
resource: Cluster/beijing
actions: ["*"]
effect: Allow
principal: {}
condition: {}
```

```yaml
name: all
resource: *
actions: ["*"]
effect: Allow
principal: {}
condition: {}
```

PermPolicyTemplate:

```yaml
name: app-manage
resource: Project/${projectName}/Application/*
actions: ["*"]
effect: Allow
principal: {}
condition: {}
```
