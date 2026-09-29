#!/bin/bash
set -e

GRAFANA_URL="${GRAFANA_URL:-http://localhost:3000}"
GRAFANA_TOKEN="${GRAFANA_TOKEN:-}"
DASHBOARD_FILE="${1:-dashboard.json}"
FOLDER_NAME="${FOLDER_NAME:-xiaozhi}"

if [ -z "$GRAFANA_TOKEN" ]; then
    echo "Error: GRAFANA_TOKEN environment variable is required"
    echo "Get a token from Grafana UI: Configuration > API Keys > New API Key"
    exit 1
fi

if [ ! -f "$DASHBOARD_FILE" ]; then
    echo "Error: Dashboard file not found: $DASHBOARD_FILE"
    exit 1
fi

echo "Creating/updating folder: $FOLDER_NAME"
FOLDER_RESPONSE=$(curl -s -X POST "$GRAFANA_URL/api/folders" \
    -H "Authorization: Bearer $GRAFANA_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"title\":\"$FOLDER_NAME\"}")

FOLDER_UID=$(echo "$FOLDER_RESPONSE" | grep -o '"uid":"[^"]*' | cut -d'"' -f4)

if [ -z "$FOLDER_UID" ]; then
    echo "Warning: Could not create folder, will try to use default"
    FOLDER_UID=""
fi

echo "Importing dashboard from $DASHBOARD_FILE..."
DASHBOARD_JSON=$(cat "$DASHBOARD_FILE")

if [ -n "$FOLDER_UID" ]; then
    PAYLOAD=$(echo "$DASHBOARD_JSON" | jq --arg folder "$FOLDER_UID" '. + {folderUid: $folder, overwrite: true}')
else
    PAYLOAD=$(echo "$DASHBOARD_JSON" | jq '{dashboard: .dashboard, overwrite: true}')
fi

IMPORT_RESPONSE=$(curl -s -X POST "$GRAFANA_URL/api/dashboards/db" \
    -H "Authorization: Bearer $GRAFANA_TOKEN" \
    -H "Content-Type: application/json" \
    -d "$PAYLOAD")

if echo "$IMPORT_RESPONSE" | grep -q '"status":"success"'; then
    echo "Dashboard imported successfully!"
    echo "$IMPORT_RESPONSE" | jq '{url: " \(.url)", uid: .dashboard.uid}'
else
    echo "Error importing dashboard:"
    echo "$IMPORT_RESPONSE"
    exit 1
fi
