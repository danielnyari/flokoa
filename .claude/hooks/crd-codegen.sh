#!/bin/bash
# PostToolUse hook: Auto-regenerate CRDs after editing Go type definitions
#
# When *_types.go files in operator/api/v1alpha1/ are modified, this hook
# runs `make manifests generate` to regenerate:
#   - CRD YAML manifests (config/crd/bases/)
#   - DeepCopy method implementations (zz_generated.deepcopy.go)
#
# This is a critical step that's easy to forget. Per the operator CLAUDE.md:
# "After modifying any *_types.go file, always run make manifests generate."

INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')

# Only trigger for *_types.go files in operator/api/v1alpha1/
if [[ "$FILE_PATH" == *operator/api/v1alpha1/*_types.go ]]; then
  OPERATOR_DIR="$CLAUDE_PROJECT_DIR/operator"

  if [ -f "$OPERATOR_DIR/Makefile" ]; then
    echo "CRD types changed ($FILE_PATH), regenerating manifests and deepcopy..." >&2
    make -C "$OPERATOR_DIR" manifests generate 2>&1
    RESULT=$?
    if [ $RESULT -ne 0 ]; then
      echo "Warning: make manifests generate failed (exit $RESULT)" >&2
    else
      echo "CRD manifests and deepcopy methods regenerated successfully." >&2
    fi
  fi
fi

exit 0
