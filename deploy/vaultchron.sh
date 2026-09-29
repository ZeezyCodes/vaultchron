#!/usr/bin/env bash
#
# vaultchron.sh — Safe wrapper for cron / systemd execution.
#
# Exports environment variables from ~/.config/vaultchron/env, navigates to the
# binary location, and executes vaultchron with the production config.
# Adjust VAULTCHRON_HOME or edit the default installation path below.
#
set -euo pipefail

# --------------------------------------------------------------------------- #
# Environment
# --------------------------------------------------------------------------- #
ENV_FILE="${HOME}/.config/vaultchron/env"
LOG_DIR="${HOME}/.config/vaultchron/logs"
LOG_FILE="${LOG_DIR}/vaultchron.log"

# Ensure log directory exists.
mkdir -p "$LOG_DIR"

# Export environment variables (e.g. GOOGLE_API_KEY) from env file if present.
if [[ -f "$ENV_FILE" ]]; then
    set -a
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    set +a
fi

# --------------------------------------------------------------------------- #
# Execution
# --------------------------------------------------------------------------- #
# Installation directory — adjust to your deployment location (e.g. /opt/vaultchron).
VAULTCHRON_DIR="${VAULTCHRON_HOME:-/opt/vaultchron}"
cd "$VAULTCHRON_DIR"

# Redirect all output to the log file.
exec >> "$LOG_FILE" 2>&1

echo "=== vaultchron run started at $(date -u '+%Y-%m-%dT%H:%M:%SZ') ==="

if ./vaultchron -config "$VAULTCHRON_DIR/config.yaml"; then
    EXIT_CODE=0
else
    EXIT_CODE=$?
fi

echo "=== vaultchron run finished at $(date -u '+%Y-%m-%dT%H:%M:%SZ') with exit code $EXIT_CODE ==="
exit $EXIT_CODE
