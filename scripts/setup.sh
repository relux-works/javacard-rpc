#!/usr/bin/env zsh
# javacard-rpc skill setup
#
# Installs:
#   1. jcrpc-gen CLI → ~/.local/bin/
#   2. javacard-rpc skill → ~/.agents/skills/ + symlinks to .claude/ and .codex/
#
# Usage: ./scripts/setup.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SKILL_NAME="javacard-rpc"

# Paths
BIN_DIR="$HOME/.local/bin"
AGENTS_SKILL_DIR="$HOME/.agents/skills/$SKILL_NAME"
CLAUDE_SKILL_DIR="$HOME/.claude/skills"
CODEX_SKILL_DIR="$HOME/.codex/skills"

echo "=== javacard-rpc setup ==="
echo ""

# --- 1. Build codegen CLI ---

echo "[1/3] Building jcrpc-gen..."
cd "$PROJECT_DIR/codegen"
go build -o jcrpc-gen ./cmd/jcrpc-gen
echo "  Built: $PROJECT_DIR/codegen/jcrpc-gen"

# --- 2. Install binary ---

echo "[2/3] Installing CLI to $BIN_DIR..."
mkdir -p "$BIN_DIR"

# Symlink binary
ln -sf "$PROJECT_DIR/codegen/jcrpc-gen" "$BIN_DIR/jcrpc-gen"
echo "  Symlinked: $BIN_DIR/jcrpc-gen → $PROJECT_DIR/codegen/jcrpc-gen"

# Verify
if command -v jcrpc-gen &>/dev/null; then
    echo "  OK: jcrpc-gen is in PATH"
else
    echo "  WARNING: $BIN_DIR is not in PATH. Add it:"
    echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
fi

# --- 3. Register skill ---

echo "[3/3] Registering skill..."

# Copy skill to ~/.agents/skills/
mkdir -p "$AGENTS_SKILL_DIR"
rsync -a --delete \
    "$PROJECT_DIR/SKILL.md" \
    "$AGENTS_SKILL_DIR/" 2>/dev/null || cp "$PROJECT_DIR/SKILL.md" "$AGENTS_SKILL_DIR/" 2>/dev/null || true

# Copy references if they exist
if [[ -d "$PROJECT_DIR/references" ]]; then
    rsync -a --delete "$PROJECT_DIR/references/" "$AGENTS_SKILL_DIR/references/" 2>/dev/null || \
        cp -r "$PROJECT_DIR/references" "$AGENTS_SKILL_DIR/" 2>/dev/null || true
fi

# Copy spec if it exists
if [[ -d "$PROJECT_DIR/.spec" ]]; then
    rsync -a --delete "$PROJECT_DIR/.spec/" "$AGENTS_SKILL_DIR/.spec/" 2>/dev/null || \
        cp -r "$PROJECT_DIR/.spec" "$AGENTS_SKILL_DIR/" 2>/dev/null || true
fi

echo "  Installed to: $AGENTS_SKILL_DIR"

# Symlink for Claude Code
mkdir -p "$CLAUDE_SKILL_DIR"
ln -sf "$AGENTS_SKILL_DIR" "$CLAUDE_SKILL_DIR/$SKILL_NAME"
echo "  Symlinked: $CLAUDE_SKILL_DIR/$SKILL_NAME"

# Symlink for Codex CLI
mkdir -p "$CODEX_SKILL_DIR"
ln -sf "$AGENTS_SKILL_DIR" "$CODEX_SKILL_DIR/$SKILL_NAME"
echo "  Symlinked: $CODEX_SKILL_DIR/$SKILL_NAME"

echo ""
echo "=== Done ==="
echo ""
echo "Usage:"
echo "  jcrpc-gen --help"
echo "  jcrpc-gen --all --out-dir . counter.toml"
