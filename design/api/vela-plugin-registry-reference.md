# KubeVela Plugin Registry Reference

## registry interface intro

`Registry` interface definitions have two methods: `GetCap` and `ListCaps`. Here "Cap" can represent trait or component definition

```go
// Registry define a registry stores trait & component defs
type Registry interface {
	GetCap(addonName string) (types.Capability, []byte, error)
	ListCaps() ([]types.Capability, error)
}
```

Now we have implemented:`GithubRegistry`, `LocalRegistry` and `OssRegistry`, which represent three types of registry source. 

To create a `Registry` object you should and must call `NewRegistry()`. 

### Helper type or function

You could use `RegistryFile` to convert `[]byte` (easily got from all kinds of source) to `Capability`. Here is `RegistryFile`'s definition.

```go
// RegistryFile describes a file item in registry
type RegistryFile struct {
	data []byte // file content
	name string // file's name
}
```

## Registry vs Capcenter

How they differ from each other? `Capcenter` is a type for vela CLI. It has some function to sync content from remote and local `~/.vela` directory and apply some `ComponentDefinition` or `TraitDefinition` to the cluster. In the contrast, `Registry` is a type for kubectl vela plugin, which focuses on cluster directly. In one word, `Registry` has no local operations. They share some basic function.