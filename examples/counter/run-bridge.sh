#!/bin/bash
# Run the javacard-rpc bridge with the Counter applet loaded.
# Usage: ./run-bridge.sh [--port 9025]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BRIDGE_DIR="$(cd "$SCRIPT_DIR/../../bridge" && pwd)"
APPLET_DIR="$SCRIPT_DIR/applet"
COUNTER_SERVER_DIR="$SCRIPT_DIR/generated/counter-server-javacard"

if [ "${JCRPC_SKIP_BUILD:-0}" != "1" ]; then
  echo "[run-bridge] building bridge..."
  (cd "$BRIDGE_DIR" && ./gradlew build --no-daemon --max-workers=2 -q) || exit 1
  echo "[run-bridge] building counter applet..."
  (cd "$APPLET_DIR" && ./gradlew build --no-daemon --max-workers=2 -q) || exit 1
fi

# Validate the build-published classpath before any bridge JVM is started.
refuse() {
  echo "[run-bridge] $1: $2; run make build-bridge" >&2
  exit 2
}

LAUNCH_DIR="$BRIDGE_DIR/build/launch"
validate_bridge_build() {
  if [ ! -s "$LAUNCH_DIR/classpath.txt" ] || [ ! -s "$LAUNCH_DIR/checksums.sha256" ]; then
    refuse bridge-build-metadata-missing "build/launch metadata is missing or empty"
  fi
  if [ "$(tail -n 1 "$LAUNCH_DIR/checksums.sha256")" != "# end of bridge build checksums" ]; then
    refuse bridge-build-metadata-invalid "incomplete build checksums"
  fi

  IFS= read -r BRIDGE_JAR < "$LAUNCH_DIR/classpath.txt" || refuse bridge-build-metadata-invalid "cannot read bridge archive"
  shopt -s nullglob
  BRIDGE_JARS=("$BRIDGE_DIR"/build/libs/*.jar)
  if [ "${#BRIDGE_JARS[@]}" -eq 0 ]; then
    refuse bridge-artifact-missing "no built bridge archive"
  fi
  if [ "${#BRIDGE_JARS[@]}" -ne 1 ]; then
    refuse bridge-artifact-ambiguous "multiple bridge archives; clean bridge/build/libs and rebuild"
  fi
  if [ "$BRIDGE_JAR" != "${BRIDGE_JARS[0]}" ]; then
    refuse bridge-artifact-stale "archive differs from the current build metadata"
  fi
  if ! shasum -a 256 -c "$LAUNCH_DIR/checksums.sha256" >/dev/null 2>&1; then
    refuse bridge-artifact-stale "build inputs, archive or runtime classpath changed since build"
  fi
}
validate_bridge_build

FULL_CP=""
while IFS= read -r entry; do
  [ -n "$entry" ] || refuse bridge-classpath-missing "runtime classpath entry is empty"
  FULL_CP="${FULL_CP:+$FULL_CP:}$entry"
done < "$LAUNCH_DIR/classpath.txt"

FULL_CP="$FULL_CP:$APPLET_DIR/build/libs/counter-applet-0.1.0.jar"
FULL_CP="$FULL_CP:$COUNTER_SERVER_DIR/build/libs/counter-server-javacard-1.0.0.jar"

echo "[run-bridge] starting bridge..."
exec java --add-modules java.smartcardio \
    -cp "$FULL_CP" \
    io.jcrpc.bridge.Main \
    --config "$SCRIPT_DIR/bridge.properties" \
    "$@"
