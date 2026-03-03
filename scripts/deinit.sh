#!/usr/bin/env zsh
# javacard-rpc skill cleanup
#
# Removes:
#   1. jcrpc-gen symlink from ~/.local/bin/
#   2. Skill registration from ~/.agents/skills/, .claude/skills/, .codex/skills/
#
# Usage: ./scripts/deinit.sh

set -euo pipefail

SKILL_NAME="javacard-rpc"
BIN_DIR="$HOME/.local/bin"

echo "=== javacard-rpc cleanup ==="

# Remove binary symlink
if [[ -L "$BIN_DIR/jcrpc-gen" ]]; then
    rm "$BIN_DIR/jcrpc-gen"
    echo "Removed: $BIN_DIR/jcrpc-gen"
fi

# Remove skill symlinks
for dir in "$HOME/.claude/skills" "$HOME/.codex/skills"; do
    if [[ -L "$dir/$SKILL_NAME" ]]; then
        rm "$dir/$SKILL_NAME"
        echo "Removed: $dir/$SKILL_NAME"
    fi
done

# Remove skill copy
if [[ -d "$HOME/.agents/skills/$SKILL_NAME" ]]; then
    rm -rf "$HOME/.agents/skills/$SKILL_NAME"
    echo "Removed: $HOME/.agents/skills/$SKILL_NAME"
fi

echo "Done."
