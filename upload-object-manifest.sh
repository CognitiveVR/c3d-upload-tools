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

# ANSI color codes
COLOR_RESET="\033[0m"
COLOR_INFO="\033[1;34m"
COLOR_WARN="\033[1;33m"
COLOR_ERROR="\033[1;31m"
COLOR_DEBUG="\033[0;36m"

# Logging helpers
log_info()  { echo -e "${COLOR_INFO}[INFO] $1${COLOR_RESET}"; }
log_warn()  { echo -e "${COLOR_WARN}[WARN] $1${COLOR_RESET}"; }
log_error() { echo -e "${COLOR_ERROR}[ERROR] $1${COLOR_RESET}"; }
log_debug() { [ "$verbose" = true ] && echo -e "${COLOR_DEBUG}[DEBUG] $1${COLOR_RESET}"; }

# ------------------
# Argument Parsing
# ------------------
VERBOSE=0
SCENE_ID=""
ENV=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --scene_id)
      SCENE_ID="$2"
      shift 2
      ;;
    --env)
      ENV="$2"
      shift 2
      ;;
    --verbose)
      VERBOSE=1
      shift
      ;;
    --help|-h)
      echo "Usage: $0 --scene_id <scene_id> --env <prod|dev> [--verbose]"
      echo "  --scene_id   (required)"
      echo "  --env        'prod' or 'dev' (required)"
      echo "  --verbose    verbose mode (optional)"
      exit 0
      ;;
    *)
      usage
      ;;
  esac
done

# ---------------------
# Validate Parameters
# ---------------------
if [[ -z "$SCENE_ID" || -z "$ENV" ]]; then
  log_error "Missing required parameter 'scene_id'."
  usage
fi

if [[ "$ENV" != "prod" && "$ENV" != "dev" ]]; then
  log_error "Environment must be 'prod' or 'dev'."
  usage
fi

# --------------------------
# Validate Required Tools
# --------------------------
for cmd in curl jq; do
  if ! command -v $cmd &>/dev/null; then
    log_error "Required command '$cmd' not found. Please install it."
    exit 1
  fi
done

# ---------------------------
# Validate API Key
# ---------------------------
if [[ -z "$C3D_DEVELOPER_API_KEY" ]]; then
  log_error "Environment variable C3D_DEVELOPER_API_KEY is not set."
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
  log_error "JSON file '$MANIFEST_FILE' does not exist."
  exit 1
fi

# --------------------
# Execute POST Request
# --------------------
if [[ "$VERBOSE" -eq 1 ]]; then
  log_info "Uploading object manifest..."
  log_info "Scene ID: $SCENE_ID"
  log_info "Environment: $ENV"
  log_info "Endpoint: $ENDPOINT"
  log_info "Manifest File: $MANIFEST_FILE"
fi

RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$ENDPOINT" \
  -H "Content-Type: application/json" \
  -H "Authorization: APIKEY:DEVELOPER $C3D_DEVELOPER_API_KEY" \
  --data-binary "@$MANIFEST_FILE")

HTTP_STATUS=$(echo "$RESPONSE" | tail -n1)
HTTP_BODY=$(echo "$RESPONSE" | sed '$d')

if [[ "$HTTP_STATUS" -ge 200 && "$HTTP_STATUS" -lt 300 ]]; then
  log_info "Upload successful. Status: $HTTP_STATUS"
  [[ "$VERBOSE" -eq 1 ]] && echo "[RESPONSE] $HTTP_BODY"
else
  log_error "Upload failed with status $HTTP_STATUS"
  log_error "$HTTP_BODY"
  exit 1
fi
