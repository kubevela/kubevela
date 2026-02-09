# Production Considerations for Global Policies

This document outlines important considerations when deploying global policies in production.

## Security Considerations

### 1. RBAC and Access Control

**Risk**: Global policies in `vela-system` apply to ALL Applications across ALL namespaces.

**Recommendations**:
```yaml
# Restrict who can create/modify global policies
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: global-policy-admin
rules:
- apiGroups: ["core.oam.dev"]
  resources: ["policydefinitions"]
  verbs: ["create", "update", "patch", "delete"]
  # IMPORTANT: Scope this carefully
---
# Only platform admins should have this role
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: platform-admins-global-policies
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: global-policy-admin
subjects:
- kind: Group
  name: platform-admins  # Restrict to platform team
  apiGroup: rbac.authorization.k8s.io
```

### 2. CUE Template Security

**Risk**: Malicious CUE templates could:
- Read sensitive data via API calls in CUE
- Cause performance issues with expensive operations
- Inject malicious values into Applications

**Recommendations**:
- **Code review all global policies** before deployment
- **Test policies in staging** with real workloads
- **Monitor CUE rendering time** - add timeouts if needed
- **Restrict CUE imports** - review what packages policies use

### 3. Namespace Isolation

**Risk**: Namespace policies could be used for privilege escalation.

**Recommendations**:
- **Audit namespace policies** - platform team should review
- **Use ResourceQuotas** to limit policy impact
- **Monitor policy changes** with admission webhooks

## Performance Considerations

### 1. Cache Hit Rates

**Monitor cache effectiveness**:
```bash
# Check cache size periodically
kubectl get app -A -o json | jq '.items | length'

# If cache hit rate is low, consider:
# 1. Increasing TTL (currently 1 minute)
# 2. Adding cache warming on policy changes
# 3. Profiling CUE rendering performance
```

### 2. CUE Rendering Performance

**Risk**: Complex CUE templates with API calls can slow reconciliation.

**Recommendations**:
- **Profile policy rendering** in staging
- **Avoid API calls in CUE** if possible
- **Use caching** for expensive computations
- **Set rendering timeouts** (future enhancement)

### 3. Large Numbers of Global Policies

**Risk**: Too many global policies slow down reconciliation.

**Recommendations**:
- **Limit global policies** - aim for <10 per namespace
- **Consolidate policies** where possible
- **Use priority** to short-circuit expensive policies

## Observability

### 1. Monitoring Policy Application

**Check which policies were applied**:
```bash
# View applied global policies
kubectl get app my-app -o jsonpath='{.status.appliedGlobalPolicies}' | jq

# View Kubernetes Events
kubectl get events --field-selector involvedObject.name=my-app

# Check for skipped policies
kubectl get app my-app -o json | jq '.status.appliedGlobalPolicies[] | select(.applied == false)'
```

### 2. Metrics to Add (Future)

```go
// Recommended metrics to add:
policyRenderDuration := prometheus.NewHistogram(...)
policyApplicationErrors := prometheus.NewCounter(...)
globalPolicyCacheHitRate := prometheus.NewGauge(...)
globalPolicyCacheSize := prometheus.NewGauge(...)
```

### 3. Debugging

**Enable verbose logging**:
```bash
# Set log level for application controller
--zap-log-level=2

# Watch for policy-related logs
kubectl logs -n vela-system deployment/vela-core | grep -i "policy\|transform"
```

## Error Handling

### 1. Current Behavior

- **Discovery failures**: Logged, reconciliation continues
- **Rendering failures**: Logged, policy skipped, reconciliation continues
- **Transform failures**: **Reconciliation FAILS** (by design)

### 2. Best Practices

**Test policies before deployment**:
```bash
# 1. Create test namespace
kubectl create ns policy-test

# 2. Deploy policy to test namespace
kubectl apply -f my-global-policy.yaml

# 3. Test with sample Application
kubectl apply -f test-app.yaml

# 4. Verify transforms applied correctly
kubectl get app test-app -o yaml

# 5. If working, promote to vela-system
kubectl apply -f my-global-policy.yaml -n vela-system
```

### 3. Rollback Strategy

If a global policy causes issues:
```bash
# Option 1: Delete the policy (cache invalidates automatically)
kubectl delete policydefinition bad-policy -n vela-system

# Option 2: Namespace override (quick fix for specific namespace)
# Create policy with same name in affected namespace
kubectl apply -f override-policy.yaml -n affected-namespace

# Option 3: Disable for specific apps
kubectl annotate app my-app policy.oam.dev/skip-global=true
```

## Operational Best Practices

### 1. Change Management

**Process for deploying global policies**:

1. **Development**:
   - Write policy in test namespace
   - Test with sample Applications
   - Code review by platform team

2. **Staging**:
   - Deploy to staging vela-system
   - Monitor for 24-48 hours
   - Check cache hit rates, error rates

3. **Production**:
   - Deploy during maintenance window
   - Monitor Application reconciliation times
   - Have rollback plan ready

### 2. Documentation

**Document each global policy**:
```yaml
apiVersion: core.oam.dev/v1beta1
kind: PolicyDefinition
metadata:
  name: security-hardening
  namespace: vela-system
  annotations:
    policy.oam.dev/description: "Adds required security labels for compliance"
    policy.oam.dev/owner: "platform-team@company.com"
    policy.oam.dev/documentation: "https://wiki.company.com/security-labels"
    policy.oam.dev/version: "v1.2.0"
    policy.oam.dev/changelog: "Added PCI-DSS compliance labels"
spec:
  # ...
```

### 3. Testing Strategy

**Recommended test pyramid**:

1. **Unit tests**: Test policy CUE templates in isolation
2. **Integration tests**: Test with real controller (use e2e tests)
3. **Staging validation**: Real workloads for 24-48 hours
4. **Gradual rollout**: Use namespace policies first, then vela-system

### 4. Migration Path

**For existing deployments**:

```bash
# 1. Enable feature gate
--feature-gates=EnableGlobalPolicies=true

# 2. Deploy policies to test namespaces first (not vela-system)
kubectl apply -f policies/ -n test-namespace

# 3. Validate with opt-in approach
# Start with Applications that explicitly want global policies
kubectl annotate app opted-in-app policy.oam.dev/skip-global=false

# 4. Gradually expand to more namespaces
# 5. Finally deploy to vela-system (cluster-wide)
```

## Known Limitations

### 1. No Dependency Ordering

Global policies are applied in priority order, but there's no explicit dependency graph.

**Workaround**: Use priority to control order
```yaml
# Base policy (runs first)
spec:
  priority: 1000

# Policy that depends on base (runs second)
spec:
  priority: 900
```

### 2. No Dry-Run Mode

Currently no way to preview what a global policy would do without applying it.

**Workaround**: Test in separate namespace first

### 3. Cache Invalidation Delay

Cache has 1-minute TTL. Policy changes may take up to 1 minute to take effect on next reconciliation.

**Workaround**: Manually trigger reconciliation if needed:
```bash
kubectl annotate app my-app reconcile.oam.dev/trigger="$(date +%s)" --overwrite
```

### 4. No Policy Ordering Within Same Priority

Policies with same priority are ordered alphabetically. No way to specify sub-ordering.

**Workaround**: Use priority values: 100, 90, 80 instead of all 100

## Security Review Checklist

Before deploying a global policy to production:

- [ ] Code reviewed by platform team
- [ ] Tested in isolated namespace
- [ ] CUE template reviewed for security issues
- [ ] No sensitive data exposed
- [ ] No expensive operations (API calls, large loops)
- [ ] Documentation added (description, owner, changelog)
- [ ] Rollback plan documented
- [ ] Monitoring/alerts configured
- [ ] RBAC reviewed (who can modify this policy?)
- [ ] Impact analysis done (how many apps affected?)

## Future Enhancements to Consider

### 1. Admission Webhooks

Add validating webhook for PolicyDefinitions:
- Prevent invalid CUE templates
- Enforce naming conventions
- Validate security policies

### 2. Policy Dry-Run

```bash
kubectl vela policy dry-run -f my-policy.yaml -n vela-system
```

### 3. Metrics and Dashboards

Grafana dashboard showing:
- Cache hit rate per policy
- Rendering duration per policy
- Number of Applications affected
- Policy application errors

### 4. Policy Dependencies

```yaml
spec:
  dependsOn:
    - security-base
    - networking-config
```

### 5. Policy Testing Framework

```bash
kubectl vela policy test -f my-policy.yaml --test-cases test-cases.yaml
```

## Getting Help

If you encounter issues:

1. **Check logs**: Application controller logs in vela-system
2. **Check status**: `kubectl get app <name> -o jsonpath='{.status.appliedGlobalPolicies}'`
3. **Check events**: `kubectl get events --field-selector involvedObject.name=<name>`
4. **Disable globally**: Delete the policy or add opt-out annotation
5. **Report issues**: File issue with policy template, Application spec, and logs

## References

- [Policy Transform README](./README.md)
- [Examples](./examples/)
- [KubeVela Documentation](https://kubevela.io)
