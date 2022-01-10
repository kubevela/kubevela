# KubeVela.io CLI Reference Doc

1. step up these two projects in the same folder.

```shell
$ tree -L 1
.
├── kubevela
└── kubevela.io
```

2. Run generate command in kubevela root dir.

```shell
cd kubevela/
go run ./hack/docgen/gen.go
```

3. Update more docs such as i18n zh

```shell
$ go run ./hack/docgen/gen.go ../kubevela.io/i18n/zh/docusaurus-plugin-content-docs/current/cli
scanning rootPath of CLI docs for replace:  ../kubevela.io/i18n/zh/docusaurus-plugin-content-docs/current/cli
```

4. Then you can check the difference in kubevela.io.

```shell
cd ../kubevela.io
git status
```