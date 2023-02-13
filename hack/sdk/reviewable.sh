# This script helps to lint and format the code in the SDK scaffold.

GO_SCAFFOLD_DIR=$(pwd)/pkg/definition/gen_sdk/_scaffold/go

pushd ${GO_SCAFFOLD_DIR}

mv go.mod_ go.mod

go mod tidy

mv go.mod go.mod_

popd

git diff --quiet

