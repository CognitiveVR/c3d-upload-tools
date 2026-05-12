#!/bin/bash

# list-objects.sh - List dynamic objects for a given scene from the Cognitive3D API

set -e

# Source shared utilities and load .env file
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/upload-utils.sh"

# Load environment variables from .env file (if present)
load_env_file

# --- Default values ---
VERBOSE=false
SCENE_ID=""
ENV=""

usage() {
  echo "Usage: $0 [--scene_id <scene_id>] [--env <prod|dev>] [--verbose]"
  echo "  --scene_id   Scene ID (or set C3D_SCENE_ID environment variable)"
  echo "  --env        Either 'prod' or 'dev' (or set C3D_ENV environment variable)"
  echo "  --verbose    Enable verbose logging"
  echo
  echo "Environment Variables:"
  echo "  C3D_DEVELOPER_API_KEY   Your Cognitive3D developer API key"
  echo "  C3D_SCENE_ID            Default scene ID (avoids --scene_id parameter)"
  echo "  C3D_ENV                 Default environment (avoids --env parameter)"
  echo "  C3D_DEFAULT_ENVIRONMENT Alternative name for default environment"
  exit 1
}

# --- Check Dependencies ---
check_dependencies

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
    *)
      log_error "Unknown option: $1"
      usage
      ;;
  esac
done

# --- Use environment variable fallback for SCENE_ID ---
if [ -z "$SCENE_ID" ]; then
  SCENE_ID="${C3D_SCENE_ID:-}"
  if [ -n "$SCENE_ID" ]; then
    log_debug "Using C3D_SCENE_ID from environment: $SCENE_ID"
  fi
fi

# --- Use environment variable fallback for ENV ---
if [ -z "$ENV" ]; then
  ENV="${C3D_ENV:-${C3D_DEFAULT_ENVIRONMENT:-}}"
  if [ -n "$ENV" ]; then
    if [ -n "${C3D_ENV:-}" ]; then
      log_debug "Using C3D_ENV from environment: $ENV"
    else
      log_debug "Using C3D_DEFAULT_ENVIRONMENT from environment: $ENV"
    fi
  fi
fi

# --- Validate Required Parameters ---
if [ -z "$SCENE_ID" ] || [ -z "$ENV" ]; then
  usage
fi

validate_uuid_format "$SCENE_ID" "scene_id"
validate_environment "$ENV"
validate_api_key

# --- Determine Base URL ---
SCENES_BASE_URL=$(get_api_base_url "$ENV" "scenes")
VERSIONS_BASE_URL=$(get_api_base_url "$ENV" "versions")

# --- Get Scene Details ---
SCENE_URL="$SCENES_BASE_URL/$SCENE_ID"
log_debug "Requesting scene details from $SCENE_URL"
SCENE_RESPONSE=$(curl -s -w "\n%{http_code}" --location \
  --header "Authorization: APIKEY:DEVELOPER $C3D_DEVELOPER_API_KEY" \
  "$SCENE_URL")

parse_http_response "$SCENE_RESPONSE"

if [ "$HTTP_STATUS" -ne 200 ]; then
  handle_http_error "$HTTP_STATUS" "$HTTP_BODY" "Get scene info"
fi

# --- Extract latest version id ---
VERSION_ID=$(echo "$HTTP_BODY" | jq -r '.versions | max_by(.versionNumber) | .id')

if [ -z "$VERSION_ID" ] || [ "$VERSION_ID" == "null" ]; then
  log_error "Could not extract version ID from scene response."
  exit 1
fi

log_debug "Resolved latest sceneVersionId = $VERSION_ID"

# --- Full URL for objects ---
URL="$VERSIONS_BASE_URL/$VERSION_ID/objects"
log_debug "Requesting objects from $URL"

# --- CURL Request ---
RESPONSE=$(curl -s -w "\n%{http_code}" --location \
  --header "Authorization: APIKEY:DEVELOPER $C3D_DEVELOPER_API_KEY" \
  "$URL")

parse_http_response "$RESPONSE"

if [ "$HTTP_STATUS" -ne 200 ]; then
  handle_http_error "$HTTP_STATUS" "$HTTP_BODY" "List objects"
fi

# --- Output JSON ---
echo "Scene Objects:"
echo "$HTTP_BODY" | jq '.'

# --- Write Raw Response to File ---
OUTPUT_FILE="${SCENE_ID}_object_list.json"
echo "$HTTP_BODY" | jq '.' > "$OUTPUT_FILE"
log_debug "Wrote raw output to $OUTPUT_FILE"

# --- Write Formatted Manifest File ---
MANIFEST_FILE="${SCENE_ID}_object_manifest.json"
echo "$HTTP_BODY" | jq '{objects: [.[] | {
  id: .sdkId,
  mesh: .meshName,
  name: .name,
  scaleCustom: (.scaleCustom // [1.0, 1.0, 1.0]),
  initialPosition: (.initialPosition // [0.0, 0.0, 0.0]),
  initialRotation: (.initialRotation // [0.0, 0.0, 0.0, 1.0])
}]}' > "$MANIFEST_FILE"
log_debug "Wrote formatted manifest to $MANIFEST_FILE"
