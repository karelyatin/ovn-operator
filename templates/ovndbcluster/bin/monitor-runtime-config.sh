#!/usr/bin/env bash
#
# Copyright 2026 Red Hat Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may
# not use this file except in compliance with the License. You may obtain
# a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and limitations
# under the License.

# Monitor for runtime configuration changes and apply them without pod restart

set -e

# These values are templated by the operator
DB_TYPE="{{ .DB_TYPE }}"
SOCKET_PATH="/tmp/ovn${DB_TYPE}_db.ctl"
CONFIG_DIR="/etc/runtime-config"
STATE_FILE="/tmp/runtime-config-state"

# Determine DB_NAME based on type
DB_NAME="OVN_Northbound"
if [[ "${DB_TYPE}" == "sb" ]]; then
    DB_NAME="OVN_Southbound"
fi

echo "Starting runtime configuration monitor for ${SERVICE_NAME} (${DB_TYPE})"

# Function to get config data from mounted ConfigMap
get_config_value() {
    local key="$1"
    local config_file="${CONFIG_DIR}/${key}"
    if [ -f "$config_file" ]; then
        cat "$config_file" 2>/dev/null || echo ""
    else
        echo ""
    fi
}

# Function to apply runtime configuration
apply_runtime_config() {
    local election_timer="$1"
    local inactivity_probe="$2"
    local log_level="$3"
    local db_scheme="$4"
    local db_port="$5"

    echo "Applying runtime configuration: election_timer=${election_timer}, inactivity_probe=${inactivity_probe}, log_level=${log_level}"

    # Check if control socket exists
    if [ ! -S "$SOCKET_PATH" ]; then
        echo "Control socket $SOCKET_PATH not available, skipping configuration"
        return 1
    fi

    # Configure election timer (leader-only operation)
    if ovs-appctl -t "$SOCKET_PATH" cluster/change-election-timer "$DB_NAME" "$election_timer" 2>/dev/null; then
        echo "Successfully configured election timer to ${election_timer}ms on leader node $(hostname)"
        IS_LEADER=true
    else
        echo "Failed to configure election timer on $(hostname) (expected if not leader)"
        IS_LEADER=false
    fi

    # Configure log level (can run on all nodes)
    if ovn-appctl -t "$SOCKET_PATH" vlog/set "console:${log_level}" 2>/dev/null; then
        echo "Successfully configured log level to ${log_level} on $(hostname)"
    else
        echo "Failed to configure log level on $(hostname)"
    fi

    # Configure inactivity probe (leader-only operation)
    if [ "$IS_LEADER" = "true" ]; then
        echo "Configuring inactivity probe on leader"

        # Start a temporary daemon for connection configuration
        export OVN_${DB_TYPE^^}_DAEMON=$(ovn-${DB_TYPE}ctl --no-leader-only --pidfile --detach 2>/dev/null) || true

        if [ -n "$OVN_${DB_TYPE^^}_DAEMON" ]; then
            # Configure inactivity probe
            ovn-${DB_TYPE}ctl --no-leader-only --inactivity-probe="$inactivity_probe" set-connection "${db_scheme}:${db_port}:[::]" 2>/dev/null || true

            # Kill the temporary daemon
            kill $(cat $OVN_RUNDIR/ovn-${DB_TYPE}ctl.pid) 2>/dev/null || true
            unset OVN_${DB_TYPE^^}_DAEMON

            echo "Successfully configured connection parameters on leader $(hostname)"
        fi
    fi
}

# Main monitoring loop
while true; do
    # Check if runtime config directory exists (ConfigMap is mounted)
    if [ ! -d "$CONFIG_DIR" ]; then
        echo "Runtime config directory $CONFIG_DIR not found, waiting..."
        sleep 30
        continue
    fi

    # Get current configuration
    election_timer=$(get_config_value "election-timer")
    inactivity_probe=$(get_config_value "inactivity-probe")
    log_level=$(get_config_value "log-level")
    db_scheme=$(get_config_value "db-scheme")
    db_port=$(get_config_value "db-port")
    timestamp=$(get_config_value "timestamp")

    # Create current state string
    current_state="${election_timer}:${inactivity_probe}:${log_level}:${db_scheme}:${db_port}:${timestamp}"

    # Check if configuration changed
    if [ -f "$STATE_FILE" ]; then
        previous_state=$(cat "$STATE_FILE")
        if [ "$current_state" = "$previous_state" ]; then
            # No change, wait and check again
            sleep 30
            continue
        fi
    fi

    # Configuration changed, apply it
    echo "Runtime configuration change detected"
    if [ -n "$election_timer" ] && [ -n "$inactivity_probe" ] && [ -n "$log_level" ]; then
        if apply_runtime_config "$election_timer" "$inactivity_probe" "$log_level" "$db_scheme" "$db_port"; then
            # Save current state if successful
            echo "$current_state" > "$STATE_FILE"
            echo "Runtime configuration applied and state saved"
        fi
    else
        echo "Incomplete configuration data, skipping"
    fi

    # Wait before checking again
    sleep 30
done