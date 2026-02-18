#!/bin/bash
# PostToolUse hook: Auto-format Python files after Edit/Write
#
# Runs ruff format and ruff check --fix on Python files in the SDK directory.
# Skips auto-generated files in flokoa-types.

INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')

# Only process Python files in the SDK directory, skip generated types
if [[ "$FILE_PATH" == *.py ]] && [[ "$FILE_PATH" == *sdk/python/* ]] && [[ "$FILE_PATH" != */flokoa-types/src/flokoa_types/agenttool.py ]] && [[ "$FILE_PATH" != */flokoa-types/src/flokoa_types/agentcard.py ]] && [[ "$FILE_PATH" != */flokoa-types/src/flokoa_types/modelconfig.py ]] && [[ "$FILE_PATH" != */flokoa-types/src/flokoa_types/templateconfig.py ]] && [[ "$FILE_PATH" != */flokoa-types/src/flokoa_types/agentworkflow.py ]] && [[ "$FILE_PATH" != */flokoa-types/src/flokoa_types/taskconfig.py ]]; then
  if [ -f "$FILE_PATH" ]; then
    RUFF=""
    if command -v ruff >/dev/null 2>&1; then
      RUFF="ruff"
    elif [ -f "$CLAUDE_PROJECT_DIR/sdk/python/.venv/bin/ruff" ]; then
      RUFF="$CLAUDE_PROJECT_DIR/sdk/python/.venv/bin/ruff"
    fi

    if [ -n "$RUFF" ]; then
      $RUFF format "$FILE_PATH" 2>&1
      $RUFF check --fix "$FILE_PATH" 2>&1 || true
    fi
  fi
fi

exit 0
