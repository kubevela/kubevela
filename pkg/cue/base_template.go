package cue

// BaseTemplate include base info provided by KubeVela for CUE template
const BaseTemplate = `

context: {
  name: string
  config?: [...{
    name: string
    value: string
  }]
}
`
