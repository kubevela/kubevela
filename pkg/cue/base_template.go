package cue

const BaseTemplate = `

context: {
  name: string
  config?: [...{
    name: string
    value: string
  }]
}
`
