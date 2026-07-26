#!/bin/sh
set -e

UUID_FILE="/var/lib/siliconflow_asset/uuid"

if [ ! -f "$UUID_FILE" ] || [ ! -s "$UUID_FILE" ]; then
    /bin/node_exporter --generate-uuid
fi

exec /bin/node_exporter "$@"
