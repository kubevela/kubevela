# KEP-2.11: Definition Testing Framework

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)

The most significant gap for platform engineers adopting 2.0. Without a testing framework, Definition authors cannot confidently publish definitions. This should be a first-class part of the Definition authoring CLI.

## Testing Model

Each Definition type has a defined testing model:

| Definition type | Test approach |
|---|---|
| `ComponentDefinition` | Unit test rendering — given parameters, assert rendered outputs |
| `TraitDefinition` | Unit test rendering — given a component output, assert patched result |
| `PolicyDefinition` | Unit test structural impact — given an Application spec, assert policy mutations |
| `ApplicationDefinition` | Integration test — given parameters, assert fully resolved Application spec |
| `WorkflowStepDefinition` | E2E harness — mock spoke cluster or real cluster with test fixtures |

## Directory Layout

Test files live alongside the Definition CUE files:

```
postgres-database/
  metadata.cue
  outputs.cue
  health.cue
  exports.cue
  workflow.cue
  tests/
    render_test.cue       # unit tests for CUE rendering
    health_test.cue       # unit tests for isHealthy expressions
    workflow_test.cue     # workflow step assertions
```

## Test Format

A `vela definition test` CLI command runs the test suite, reports failures with diffs. The framework is CUE-native — test cases are CUE values asserting expected output shapes:

```cue
// tests/render_test.cue
testCases: [
  {
    name: "default storage"
    input: { version: "14", storage: "10Gi", dbName: "app" }
    assert: {
      outputs: [{
        name: "database"
        value: spec: template: spec: containers: [{
          image: "postgres:14"
        }]
      }]
    }
  }
]
```
