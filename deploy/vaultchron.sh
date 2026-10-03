#!/usr/bin/env bash
#
# vaultchron.sh — Safe wrapper for cron / systemd execution.
#
# Exports environment variables from ~/.config/vaultchron/env, navigates to the
# binary location, and executes vaultchron with the production config.
#
set -euo pipefail

# --------------------------------------------------------------------------- #
# Logging & Environment
# --------------------------------------------------------------------------- #
LOG_DIR="${HOME}/.config/vaultchron/logs"
LOG_FILE="${LOG_DIR}/vaultchron.log"

# Ensure log directory exists before any other action.
mkdir -p "$LOG_DIR"

# Redirect stdout and stderr of the whole script to vaultchron.log.
exec >> "$LOG_FILE" 2>&1

ENV_FILE="${HOME}/.config/vaultchron/env"
if [[ -f "$ENV_FILE" ]]; then
    set -a
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    set +a
fi

# --------------------------------------------------------------------------- #
# Execution
# --------------------------------------------------------------------------- #
if [[ -n "${VAULTCHRON_HOME:-}" ]]; then
    if [[ ! -d "$VAULTCHRON_HOME" ]]; then
        echo "vaultchron: VAULTCHRON_HOME $VAULTCHRON_HOME does not exist"
        exit 1
    fi
    cd "$VAULTCHRON_HOME"
    echo "=== vaultchron run started at $(date -u '+%Y-%m-%dT%H:%M:%SZ') ==="
    if ./vaultchron -config "$VAULTCHRON_HOME/config.yaml"; then
        EXIT_CODE=0
    else
        EXIT_CODE=$?
    fi
    echo "=== vaultchron run finished at $(date -u '+%Y-%m-%dT%H:%M:%SZ') with exit code $EXIT_CODE ==="
    exit $EXIT_CODE
else
    PATH="${HOME}/.local/bin:${PATH}"
    export PATH
    if ! command -v vaultchron >/dev/null; then
        echo "vaultchron: not found on PATH (looked in $HOME/.local/bin); install it or set VAULTCHRON_HOME"
        exit 1
    fi
    echo "=== vaultchron run started at $(date -u '+%Y-%m-%dT%H:%M:%SZ') ==="
    if vaultchron; then
        EXIT_CODE=0
    else
        EXIT_CODE=$?
    fi
    echo "=== vaultchron run finished at $(date -u '+%Y-%m-%dT%H:%M:%SZ') with exit code $EXIT_CODE ==="
    exit $EXIT_CODE
fi
