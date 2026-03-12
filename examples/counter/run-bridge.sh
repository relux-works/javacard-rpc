#!/bin/bash
# Run the javacard-rpc bridge with the Counter applet loaded.
# Usage: ./run-bridge.sh [--port 9025]

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BRIDGE_DIR="$SCRIPT_DIR/../../bridge"
APPLET_DIR="$SCRIPT_DIR/applet"
COUNTER_SERVER_DIR="$SCRIPT_DIR/generated/counter-server-javacard"

if [ "${JCRPC_SKIP_BUILD:-0}" != "1" ]; then
  echo "[run-bridge] building bridge..."
  (cd "$BRIDGE_DIR" && ./gradlew build -q) || exit 1
  echo "[run-bridge] building counter applet..."
  (cd "$APPLET_DIR" && ./gradlew build -q) || exit 1
fi

# Collect classpath
JCARDSIM_JAR=$(find ~/.gradle/caches -name "jcardsim-3.0.5.9.jar" -print -quit 2>/dev/null)
SMARTCARDIO_JAR="$BRIDGE_DIR/libs/smartcardio.jar"

FULL_CP="$BRIDGE_DIR/build/libs/jcrpc-bridge-0.1.0.jar"
FULL_CP="$FULL_CP:$APPLET_DIR/build/libs/counter-applet-0.1.0.jar"
FULL_CP="$FULL_CP:$COUNTER_SERVER_DIR/build/libs/counter-server-javacard-1.0.0.jar"
FULL_CP="$FULL_CP:$JCARDSIM_JAR:$SMARTCARDIO_JAR"

echo "[run-bridge] starting bridge..."
exec java --add-modules java.smartcardio \
    -cp "$FULL_CP" \
    io.jcrpc.bridge.Main \
    --config "$SCRIPT_DIR/bridge.properties" \
    "$@"
