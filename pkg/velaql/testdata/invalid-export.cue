parameter: {
    name: "test"
}

output: {
    message: "hello " + parameter.name
}

export: 123  // Invalid export type (should be string)
