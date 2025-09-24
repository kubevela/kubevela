kubectl apply -f - <<EOF
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: test-component
  namespace: vela-system
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  schematic:
    cue:
      template: |
        outputs: {
          invalid: {
            apiVersion: "custom.io/v1alpha1"
            kind: "NonExistentResource"
            metadata: name: "test"
          }
        }
EOF

kubectl apply -f - <<EOF
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: bad-crd-trait
  namespace: vela-system
spec:
  appliesToWorkloads:
    - deployments.apps
  schematic:
    cue:
      template: |
        outputs: {
          invalid: {
            apiVersion: "custom.io/v1alpha1"
            kind: "NonExistentResource"
            metadata: name: "test"
          }
        }
EOF