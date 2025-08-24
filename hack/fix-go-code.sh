#!/usr/bin/env bash

# This script attempts to fix common Go code issues in the project

set -e

echo "Fixing import cycles and syntax errors..."

# Find all go files with syntax errors
for file in $(find . -name "*.go" -type f); do
  # Check if the file has syntax errors
  if ! go fmt $file >/dev/null 2>&1; then
    echo "Fixing syntax in $file"
    # Just run goimports to attempt to fix
    goimports -w $file
  fi
done

echo "Checking for duplicate type declarations..."

# Fix the package application import issue
if grep -q "^import" pkg/controller/core.oam.dev/v1alpha2/application/import.go 2>/dev/null; then
  echo "Fixing import.go issue in application package"
  sed -i '1s/^/package application\n\n/' pkg/controller/core.oam.dev/v1alpha2/application/import.go
fi

# Fix the duplicate TraitDefinitionSpec issue
if [ -f pkg/apis/core.oam.dev/v1beta1/core_types_structs.go ]; then
  echo "Temporarily renaming duplicate type declarations in core_types_structs.go"
  sed -i 's/^type TraitDefinitionSpec/\/\/ Disabled: type TraitDefinitionSpec/' pkg/apis/core.oam.dev/v1beta1/core_types_structs.go
  sed -i 's/^type TraitDefinition/\/\/ Disabled: type TraitDefinition/' pkg/apis/core.oam.dev/v1beta1/core_types_structs.go
  sed -i 's/^type TraitDefinitionList/\/\/ Disabled: type TraitDefinitionList/' pkg/apis/core.oam.dev/v1beta1/core_types_structs.go
fi

echo "Fixing common syntax errors in e2e tests..."

# Fix the e2e/commonContext.go missing braces
if [ -f e2e/commonContext.go ]; then
  echo "Fixing syntax in e2e/commonContext.go"
  sed -i '/DeleteEnvFunc = func(context string, envName string) bool {/,/EnvShowContext = func/ {
    s/EnvShowContext = func/})\n\t}\n\n\tEnvShowContext = func/
  }' e2e/commonContext.go
  
  sed -i '/EnvSetContext = func(context string, envName string) bool {/,/EnvDeleteContext = func/ {
    s/EnvDeleteContext = func/})\n\t}\n\n\tEnvDeleteContext = func/
  }' e2e/commonContext.go
fi

echo "Running go mod tidy..."
go mod tidy

echo "Code fixing completed"
