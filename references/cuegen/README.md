## CUE Generator

Auto generation of CUE schema and docs from Go struct

## Type Conversion

- All comments will be copied to CUE schema

### Basic Types

|      Go Type       | CUE Type  |
|:------------------:|:---------:|
|       `int`        |   `int`   |
|       `int8`       |  `int8`   |
|      `int16`       |  `int16`  |
|      `int32`       |  `int32`  |
|      `int64`       |  `int64`  |
|       `uint`       |  `uint`   |
|      `uint8`       |  `uint8`  |
|      `uint16`      | `uint16`  |
|      `uint32`      | `uint32`  |
|      `uint64`      | `uint64`  |
|     `float32`      | `float32` |
|     `float64`      | `float64` |
|      `string`      | `string`  |
|       `bool`       |  `bool`   |
|       `nil`        |  `null`   |
|       `byte`       |  `uint8`  |
|     `uintptr`      | `uint64`  |
|      `[]byte`      |  `bytes`  |
| `interface{}/any`  |  `{...}`  |
| `interface{ ... }` |    `_`    |

### Map Type

- CUE only supports `map[string]T` type, which is converted to `[string]: T` in CUE schema
- All `map[string]any/map[string]interface{}` are converted to `{...}` in CUE schema

### Struct Type

- Fields will be expanded recursively in CUE schema
- All unexported fields will be ignored
- Do not support recursive struct type, which will cause infinite loop

`json` Tag:

- Fields with `json:"FIELD_NAME"` tag will be renamed to `FIELD_NAME` in CUE schema, otherwise the field name will be
  used
- Fields with `json:"-"` tag will be ignored in generation
- Anonymous fields with `json:",inline"` tag will be expanded inlined in CUE schema
- Fields with `json:",omitempty"` tag will be marked as optional in CUE schema

`cue` Tag:

- Format: `cue:"key1:value1;key2:value2;boolValue1;boolValue2"`
- Fields with `cue:"enum:VALUE1,VALUE2"` tag will be set with enum values `VALUE1` and `VALUE2` in CUE schema
- Fields with `cue:"default:VALUE"` tag will be set with default value `VALUE` in CUE schema, and `VALUE` must be one of
  go basic types, including `int`, `float`, `string`, `bool`
- Separators `';'`, `':'` and `','` can be escaped with `'\'`, e.g. `cue:"default:va\\;lue\\:;enum:e\\;num1,e\\:num2\\,enum3"` will
  be parsed as `Default: "va;lue:", Enum: []string{"e;num1", "e:num2,enum3"}}`
