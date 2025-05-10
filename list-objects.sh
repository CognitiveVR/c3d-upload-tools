#!/bin/bash

# list_scene_objects.sh - List objects for a given scene from the Cognitive3D API

set -e

# --- Default values ---
VERBOSE=false
DEBUG=false
SCENE_ID=""
ENV=""

# --- Helper Functions ---
log() {
  if [ "$VERBOSE" = true ]; then
    echo "[INFO] $1"
  fi
}

debug() {
  if [ "$DEBUG" = true ]; then
    echo "[DEBUG] $1"
  fi
}

usage() {
  echo "Usage: $0 --scene_id <scene_id> --env <prod|dev> [--verbose] [--debug]"
  exit 1
}

# --- Check Dependencies ---
if ! command -v jq >/dev/null 2>&1; then
  echo "Error: 'jq' is not installed. Please install it before running this script."
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "Error: 'curl' is not installed. Please install it before running this script."
  exit 1
fi

# --- Parse Arguments ---
while [[ $# -gt 0 ]]; do
  key="$1"
  case $key in
    --scene_id)
      SCENE_ID="$2"
      shift; shift
      ;;
    --env)
      ENV="$2"
      shift; shift
      ;;
    --verbose)
      VERBOSE=true
      shift
      ;;
    --debug)
      DEBUG=true
      shift
      ;;
    *)
      echo "Unknown option: $1"
      usage
      ;;
  esac
done

# --- Validate Required Parameters ---
if [ -z "$SCENE_ID" ] || [ -z "$ENV" ]; then
  usage
fi

# --- Determine Base URL ---
if [ "$ENV" == "prod" ]; then
  BASE_URL="https://data.cognitive3d.com"
elif [ "$ENV" == "dev" ]; then
  BASE_URL="https://data.c3ddev.com"
else
  echo "Invalid environment: $ENV. Must be 'prod' or 'dev'."
  exit 1
fi

# --- Developer API Key (expected to be set in ENV) ---
if [ -z "$C3D_DEVELOPER_API_KEY" ]; then
  echo "Environment variable C3D_DEVELOPER_API_KEY is not set."
  exit 1
fi

# --- Get Scene Details ---
SCENE_URL="$BASE_URL/v0/scenes/$SCENE_ID"
log "Requesting scene details from $SCENE_URL"
SCENE_RESPONSE=$(curl --silent --show-error --location \
  --header "Authorization: APIKEY:DEVELOPER $C3D_DEVELOPER_API_KEY" \
  "$SCENE_URL" -w "\nHTTP_STATUS:%{http_code}")

SCENE_BODY=$(echo "$SCENE_RESPONSE" | sed -e 's/HTTP_STATUS\:.*//g')
SCENE_STATUS=$(echo "$SCENE_RESPONSE" | tr -d '\n' | sed -e 's/.*HTTP_STATUS://')

if [ "$SCENE_STATUS" -ne 200 ]; then
  echo "Error: Failed to get scene info. HTTP $SCENE_STATUS"
  echo "$SCENE_BODY"
  exit 1
fi

# --- Extract latest version id ---
VERSION_ID=$(echo "$SCENE_BODY" | jq '.versions | max_by(.versionNumber) | .id')

if [ -z "$VERSION_ID" ] || [ "$VERSION_ID" == "null" ]; then
  echo "Error: Could not extract version ID from scene response."
  exit 1
fi

log "Resolved latest sceneVersionId = $VERSION_ID"

# --- Full URL for objects ---
URL="$BASE_URL/v0/versions/$VERSION_ID/objects"
log "Requesting objects from $URL"

# --- CURL Request ---
RESPONSE=$(curl --silent --show-error --location \
  --header "Authorization: APIKEY:DEVELOPER $C3D_DEVELOPER_API_KEY" \
  "$URL" -w "\nHTTP_STATUS:%{http_code}")

# --- Parse Response ---
BODY=$(echo "$RESPONSE" | sed -e 's/HTTP_STATUS\:.*//g')
STATUS=$(echo "$RESPONSE" | tr -d '\n' | sed -e 's/.*HTTP_STATUS://')

if [ "$STATUS" -ne 200 ]; then
  echo "Error: Received HTTP $STATUS"
  echo "$BODY"
  exit 1
fi

# --- Output JSON ---
echo "Scene Objects:"
echo "$BODY" | jq '.'

# --- Write Raw Response to File ---
OUTPUT_FILE="${SCENE_ID}_object_list.json"
echo "$BODY" | jq '.' > "$OUTPUT_FILE"
log "Wrote raw output to $OUTPUT_FILE"

# --- Write Formatted Manifest File ---
MANIFEST_FILE="${SCENE_ID}_object_manifest.json"
echo "$BODY" | jq '{objects: [.[] | {
  id: .sdkId,
  mesh: .meshName,
  name: .name,
  scaleCustom: (.scaleCustom // [1.0, 1.0, 1.0]),
  initialPosition: (.initialPosition // [0.0, 0.0, 0.0]),
  initialRotation: (.initialRotation // [0.0, 0.0, 0.0, 1.0])
}]}' > "$MANIFEST_FILE"
log "Wrote formatted manifest to $MANIFEST_FILE"
