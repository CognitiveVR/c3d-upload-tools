#!/bin/bash

# upload-object-manifest.sh
# Upload a JSON object manifest to the Cognitive3D platform

set -e

# ----------------------
# Configuration & Usage
# ----------------------
usage() {
  echo "Usage: $0 -s <scene_id> -e <env> [-v]"
  echo "  -s scene_id (required)"
  echo "  -e environment: 'prod' or 'dev' (required)"
  echo "  -v verbose mode (optional)"
  exit 1
}

# ------------------
# Argument Parsing
# ------------------
VERBOSE=0
while getopts ":s:e:v" opt; do
  case ${opt} in
    s) SCENE_ID="$OPTARG" ;;
    e) ENV="$OPTARG" ;;
    v) VERBOSE=1 ;;
    *) usage ;;
  esac
done

# ---------------------
# Validate Parameters
# ---------------------
if [[ -z "$SCENE_ID" || -z "$ENV" ]]; then
  echo "[ERROR] Missing required parameters."
  usage
fi

if [[ "$ENV" != "prod" && "$ENV" != "dev" ]]; then
  echo "[ERROR] Environment must be 'prod' or 'dev'."
  usage
fi

# --------------------------
# Validate Required Tools
# --------------------------
for cmd in curl jq; do
  if ! command -v $cmd &>/dev/null; then
    echo "[ERROR] Required command '$cmd' not found. Please install it."
    exit 1
  fi
done

# ---------------------------
# Validate API Key
# ---------------------------
if [[ -z "$C3D_DEVELOPER_API_KEY" ]]; then
  echo "[ERROR] Environment variable C3D_DEVELOPER_API_KEY is not set."
  exit 1
fi

# -------------------------
# Set API URL and JSON file
# -------------------------
BASE_URL="https://data.cognitive3d.com"
[[ "$ENV" == "dev" ]] && BASE_URL="https://data.c3ddev.com"

ENDPOINT="$BASE_URL/v0/objects/$SCENE_ID"
MANIFEST_FILE="${SCENE_ID}_object_manifest.json"

if [[ ! -f "$MANIFEST_FILE" ]]; then
  echo "[ERROR] JSON file '$MANIFEST_FILE' does not exist."
  exit 1
fi

# --------------------
# Execute POST Request
# --------------------
if [[ "$VERBOSE" -eq 1 ]]; then
  echo "[INFO] Uploading object manifest..."
  echo "[INFO] Scene ID: $SCENE_ID"
  echo "[INFO] Environment: $ENV"
  echo "[INFO] Endpoint: $ENDPOINT"
  echo "[INFO] Manifest File: $MANIFEST_FILE"
fi

RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$ENDPOINT" \
  -H "Content-Type: application/json" \
  -H "Authorization: APIKEY:DEVELOPER $C3D_DEVELOPER_API_KEY" \
  --data-binary "@$MANIFEST_FILE")

HTTP_STATUS=$(echo "$RESPONSE" | tail -n1)
HTTP_BODY=$(echo "$RESPONSE" | sed '$d')

if [[ "$HTTP_STATUS" -ge 200 && "$HTTP_STATUS" -lt 300 ]]; then
  echo "[SUCCESS] Upload successful. Status: $HTTP_STATUS"
  [[ "$VERBOSE" -eq 1 ]] && echo "[RESPONSE] $HTTP_BODY"
else
  echo "[ERROR] Upload failed with status $HTTP_STATUS"
  echo "[RESPONSE] $HTTP_BODY"
  exit 1
fi
