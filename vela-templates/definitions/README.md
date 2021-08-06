# Vela Definitions

This directory contains KubeVela definitions that could be used in Applications to extend the ability of workloads.

To install definition like **internal/resource.cue**, run `vela def apply internal/resource.cue` to install it. You can check if by running `vela def get resource` or `vela def list`. Optionally, you can run `vela up -f ./usage-examples/application-with-resource.yaml` to see how to use this **resource** definition with an application.

If you would like to customize your own *resource* definitions, you can run `vela def edit resource` or edit the **internal/resource.cue** file locally and then re-run `vela def apply internal/resource.cue`. You can also run `vela def vet internal/resource.cue` to check if your modification is valid.

If you do not want it anymore, you can run `vela def del resource` to remove it.

Finally, if you would like to create your own definition from scratch, you can run `vela def init my-definition -i` to initiate your definition.