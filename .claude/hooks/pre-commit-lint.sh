#!/bin/bash
# PreToolUse hook: Run linting checks before git commit
#
# Inspects the Bash command about to be executed. If it's a git commit,
# runs the appropriate linters based on which files are staged:
#   - Go files in operator/: make lint (golangci-lint)
#   - Python files in sdk/python/: ruff check + ty check
#
# Exits with code 2 to block the commit if linting fails.

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

# Only trigger for git commit commands
if ! echo "$COMMAND" | grep -qE '^\s*git\s+commit'; then
  exit 0
fi

FAILED=0

# Check for staged Go changes in operator/
if git -C "$CLAUDE_PROJECT_DIR" diff --cached --name-only 2>/dev/null | grep -q '^operator/.*\.go$'; then
  echo "Staged Go files detected - running operator lint..." >&2
  LINT_OUTPUT=$(make -C "$CLAUDE_PROJECT_DIR/operator" lint 2>&1)
  LINT_EXIT=$?
  if [ $LINT_EXIT -ne 0 ]; then
    # Distinguish between network/toolchain errors and real lint errors
    if echo "$LINT_OUTPUT" | grep -q "download go1\." || echo "$LINT_OUTPUT" | grep -q "dial tcp"; then
      echo "Go lint skipped: Go toolchain unavailable (network error)." >&2
    else
      echo "Go lint failed. Fix lint errors before committing." >&2
      echo "$LINT_OUTPUT" >&2
      FAILED=1
    fi
  fi
fi

# Check for staged Python changes in sdk/python/
if git -C "$CLAUDE_PROJECT_DIR" diff --cached --name-only 2>/dev/null | grep -q '^sdk/python/.*\.py$'; then
  echo "Staged Python files detected - running ruff on staged files..." >&2
  STAGED_PY=$(git -C "$CLAUDE_PROJECT_DIR" diff --cached --name-only 2>/dev/null | grep '^sdk/python/.*\.py$' | sed "s|^sdk/python/||")
  cd "$CLAUDE_PROJECT_DIR/sdk/python"
  echo "$STAGED_PY" | xargs uv run --package flokoa ruff check 2>&1
  if [ $? -ne 0 ]; then
    echo "Python lint failed on staged files. Fix lint errors before committing." >&2
    FAILED=1
  fi
fi

if [ $FAILED -ne 0 ]; then
  exit 2
fi

exit 0
