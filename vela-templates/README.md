This is the stole place for

1. Hold built-in CUE templates for Vela Core and Registry. `registry` and `internal` store these templates
   
   To update definitions in charts and registry, run:
   
   ```shell
   ./vela-templates/gen_definitions.sh
   ```
2. Hold built in addon templates.
   
   `addons` stores these templates. Each one directory of `addons` represent an addon. For one addon, the directory like:
   
   ```shell
   example-addon
   ├── definitions # component defs can be use after this addon was enabled
   │   └── example-def.yaml
   ├── resource # resources to generate Initializer
   │   ├── some-resources-dir
   │   └── other-resources-dir
   └── template.yaml # fixed filename
   ```
   
   To generate addon, run
   
   ```shell
    go run ./vela-templates/gen_addons.go --addons-path=./vela-templates/addons --store-path=./charts/vela-core/templates/addons 
   ```
   
   This will generate
      1. `charts/vela-core/addons/example-addon.yaml` (only Initializer)
      2. `vela-templates/addons/demo/example-addon.yaml` (Initializer and ComponentDefinition)