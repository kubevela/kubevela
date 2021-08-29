This is the stole place for

1. Hold built-in CUE templates for Vela Core and Registry. `definitions/registry` and `definitions/internal` store these templates
   
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
    go run ./vela-templates/gen_addons.go 
   ```
   
   This will generate
      1. `charts/vela-core/templates/addons/example-addon.yaml` (addon ConfigMap)
      2. `charts/vela-core/templates/addons-default/example-addon.yaml` (default enabled addon Initializer)
      3. `vela-templates/addons/auto-gen/example-addon.yaml` (addon Initializer)
