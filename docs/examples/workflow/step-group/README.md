# Step Group

## How to start

Edit a yaml file as `example.yaml`, then execute it with `vela up` command.

## Parameter Introduction

`step-group` has a `subSteps` parameter which is an array containing any step type whose valid parameters do not include the `step-group` step type itself.

`step-group` doesn't support `properties` for now.

## Execute process

When executing the `step-group` step, the subSteps in the step group are executed in dag mode. The step group will only complete when all subSteps have been executed to completion.
SubStep has the same execution behavior as a normal step.
