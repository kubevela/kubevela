# Example FluxCD Addon

This is an example addon based [FluxCD](https://fluxcd.io/)

## Directory Structure

- `template.yaml`: contains the basic app, you can add some component and workflow to meet your requirements. Other files 
  in `resources/` and `definitions/` will be rendered as Components and appended in `spec.components`
- `metadata.yaml`: contains addon metadata information.
- `definitions/`: contains the X-Definition yaml/cue files. These file will be rendered as KubeVela Component in `template.yaml`
- `resources/`:
  - `parameter.cue` to expose parameters. It will be converted to JSON schema and rendered in UI forms.
  - All other files will be rendered as KubeVela Components. It can be one of the two types:
    - YAML file that contains only one resource. This will be rendered as a `raw` component
    - CUE template file that can read user input as `parameter.XXX` as defined `parameter.cue`.
      Basically the CUE template file will be combined with `parameter.cue` to render a resource.
      **You can specify the type and trait in this format**
      

