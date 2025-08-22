#!/usr/bin/env bash
# Wrapper script to run golangci-lint and ignore typecheck errors
# This script filters out false positive typecheck errors from golangci-lint v1.60.1

set -euo pipefail

# Create temporary files for output
tmpfile=$(mktemp)
filtered=$(mktemp)

# Ensure cleanup on exit
cleanup() {
    rm -f "$tmpfile" "$filtered"
}
trap cleanup EXIT INT TERM

if ${GOLANGCILINT:-golangci-lint} run --config .golangci.yml --fix --verbose --exclude-dirs 'scaffold' > "$tmpfile" 2>&1; then
    exit_code=0
else
    exit_code=$?
fi

# Keep the original verbose output (level= lines)
grep -E "^level=|^[[:space:]]*$" "$tmpfile" > "$filtered" || true

# Check for non-typecheck errors and add them to output
if grep -E "\.go:[0-9]+:[0-9]+:.*\(" "$tmpfile" 2>/dev/null | grep -v "(typecheck)" > /dev/null; then
    {
        echo ""
        echo "Linting errors found:"
        grep -E "\.go:[0-9]+:[0-9]+:.*\(" "$tmpfile" | grep -v "(typecheck)" || true
    } >> "$filtered"
fi

cat "$filtered"
if [[ $exit_code -eq 0 ]]; then
    exit 0
fi

# Count non-typecheck errors in .go files
non_typecheck_errors=$(grep -E "\.go:[0-9]+:[0-9]+:.*\(" "$tmpfile" 2>/dev/null | grep -v "(typecheck)" | grep -c . || echo "0")

# Ensure the count is a valid number (remove spaces and newlines)
non_typecheck_errors=$(echo "$non_typecheck_errors" | tr -d '[:space:]')

# Check if golangci-lint itself had a critical failure (not a linting error)
# This catches cases like config errors, missing files, etc.
if [[ $exit_code -ne 0 ]] && [[ $non_typecheck_errors -eq 0 ]]; then
    # Check if there are only typecheck errors
    total_errors=$(grep -E "\.go:[0-9]+:[0-9]+:.*\(" "$tmpfile" 2>/dev/null | grep -c . || echo "0")
    typecheck_errors=$(grep -E "\.go:[0-9]+:[0-9]+:.*\(typecheck\)" "$tmpfile" 2>/dev/null | grep -c . || echo "0")
    
    # Clean up the counts
    total_errors=$(echo "$total_errors" | tr -d '[:space:]')
    typecheck_errors=$(echo "$typecheck_errors" | tr -d '[:space:]')
    
    if [[ $total_errors -eq $typecheck_errors ]]; then
        # Only typecheck errors, ignore them
        exit 0
    else
        # There was a critical failure, show the full output and exit with error
        echo ""
        echo "Critical golangci-lint failure detected:"
        cat "$tmpfile"
        exit "$exit_code"
    fi
fi

if [[ $non_typecheck_errors -gt 0 ]]; then
    exit "$exit_code"
else
    # Only typecheck errors, ignore them
    exit 0
fi