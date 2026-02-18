#!/bin/bash
# PostToolUse hook: Auto-format Go files after Edit/Write
#
# Runs gofmt on Go files edited in the operator/ directory.
# Skips generated files (zz_generated.*).

INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')

# Only process Go files in the operator directory, skip generated files
if [[ "$FILE_PATH" == *.go ]] && [[ "$FILE_PATH" == *operator/* ]] && [[ "$FILE_PATH" != *zz_generated* ]]; then
  if [ -f "$FILE_PATH" ]; then
    gofmt -w "$FILE_PATH" 2>&1
  fi
fi

exit 0
