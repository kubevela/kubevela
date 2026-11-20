# KubeVela.io CLI Reference Doc

Refer to [contribute docs in kubevela.io](http://kubevela.io/docs/contributor/cli-ref-doc) for more details.

## vela mcp tool reference

`go run ./hack/docgen/mcp` writes `docs/reference/vela-mcp-tools.md` in a kubevela.github.io
checkout next to this repository; pass another path as the first argument. The page is built
from the tool descriptions and schemas the server publishes, so change those in `pkg/mcp`, not the page.
