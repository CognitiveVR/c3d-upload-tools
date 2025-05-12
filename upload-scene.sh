#!/bin/bash

# Exit immediately if a command exits with a non-zero status.
set -e

# Treat unset variables as an error.
set -u

# Uncomment for debugging
# set -x

# Define script variables
SCRIPT_NAME="$(basename "$0")"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

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
log_debug() { [ "$VERBOSE" = true ] && echo -e "${COLOR_DEBUG}[DEBUG] $1${COLOR_RESET}"; }

# --- Check Dependencies ---
if ! command -v jq >/dev/null 2>&1; then
  log_error "'jq' is not installed. Please install it before running this script."
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  log_error "'curl' is not installed. Please install it before running this script."
  exit 1
fi

get_api_base_url() {
  local env="$1"
  case "$env" in
    prod)
      echo "https://data.cognitive3d.com/v0/scenes"
      ;;
    dev)
      echo "https://data.c3ddev.com/v0/scenes"
      ;;
    *)
      log_error "Unknown environment: $env"
      exit 1
      ;;
  esac
}

# Main function
main() {
  # Default values
  SCENE_DIRECTORY=""
  ENVIRONMENT="prod"
  SCENE_ID=""
  VERBOSE=false

  # Parse named arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --scene_dir)
        SCENE_DIRECTORY="$2"
        shift 2
        ;;
      --env)
        ENVIRONMENT="$2"
        shift 2
        ;;
      --scene_id)
        SCENE_ID="$2"
        shift 2
        ;;
      --verbose)
        VERBOSE=true
        shift
        ;;
      --help|-h)
        echo "Usage: $SCRIPT_NAME --scene_dir <scene_directory> [--env <prod|dev>] [--scene_id <scene_id>] [--verbose]"
        echo "  --scene_dir   Path to folder containing 4 files: scene.bin, scene.gltf, screenshot.png, settings.json"
        echo "  --env         Optional. Either 'prod' (default) or 'dev'"
        echo "  --scene_id    Optional. Appended to API URL if present"
        echo "  --verbose     Optional. Enables verbose output"
        echo
        echo "Environment Variables:"
        echo "  C3D_DEVELOPER_API_KEY   Your Cognitive3D developer API key"
        exit 0
        ;;
      *)
        log_error "Unknown argument: $1"
        exit 1
        ;;
    esac
  done

  # Logging helper for verbose mode
  log_verbose() {
    if [ "$VERBOSE" = true ]; then
      echo "[VERBOSE] $1"
    fi
  }

  # Validate required CLI parameter
  if [[ -z "$SCENE_DIRECTORY" ]]; then
    log_error "Missing required argument: --scene_dir"
    echo "Usage: $SCRIPT_NAME --scene_dir <scene_directory> [--env <prod|dev>] [--scene_id <scene_id>] [--verbose]"
    exit 1
  fi

  if [[ ! -d "$SCENE_DIRECTORY" ]]; then
    log_error "The specified scene directory does not exist: $SCENE_DIRECTORY"
    exit 1
  fi

  if [[ "$ENVIRONMENT" != "prod" && "$ENVIRONMENT" != "dev" ]]; then
    log_error "Invalid environment: $ENVIRONMENT. Must be 'prod' or 'dev'."
    exit 1
  fi

  log_info "Using environment: $ENVIRONMENT"
  [[ -n "$SCENE_ID" ]] && log_info "Using scene ID: $SCENE_ID"
  log_verbose "SCENE_DIRECTORY: $SCENE_DIRECTORY"

  # Import environment variable
  if [[ -z "${C3D_DEVELOPER_API_KEY:-}" ]]; then
    log_error "C3D_DEVELOPER_API_KEY is not set. Please set it with: export C3D_DEVELOPER_API_KEY=your_api_key"
    exit 1
  fi

  local C3D_DEVELOPER_API_KEY="$C3D_DEVELOPER_API_KEY"
  log_info "C3D_DEVELOPER_API_KEY has been set."
  log_info "SCENE_DIRECTORY is: $SCENE_DIRECTORY"

  # Determine API base URL
  local BASE_URL
  BASE_URL=$(get_api_base_url "$ENVIRONMENT")
  if [[ -n "$SCENE_ID" ]]; then
    BASE_URL+="/$SCENE_ID"
  fi
  log_info "Using API base URL: $BASE_URL"

  # Prepare file paths
  local BIN_FILE="$SCENE_DIRECTORY/scene.bin"
  local GLTF_FILE="$SCENE_DIRECTORY/scene.gltf"
  local PNG_FILE="$SCENE_DIRECTORY/screenshot.png"
  local JSON_FILE="$SCENE_DIRECTORY/settings.json"

  # Validate file existence
  for file in "$BIN_FILE" "$GLTF_FILE" "$PNG_FILE" "$JSON_FILE"; do
    if [[ ! -f "$file" ]]; then
      log_error "Required file missing: $file"
      exit 1
    fi
  done

  # Read sdk-version.txt
  local SDK_VERSION_FILE="$SCRIPT_DIR/sdk-version.txt"
  if [[ ! -s "$SDK_VERSION_FILE" ]]; then
    log_error "sdk-version.txt is missing or empty at: $SDK_VERSION_FILE"
    exit 1
  fi
  local SDK_VERSION
  SDK_VERSION=$(cat "$SDK_VERSION_FILE")
  log_info "Read SDK version: $SDK_VERSION"

  # Update settings.json with new sdkVersion using jq
  local TMP_JSON_FILE="$SCENE_DIRECTORY/settings-updated.json"
  jq --arg sdk "$SDK_VERSION" '.sdkVersion = $sdk' "$JSON_FILE" > "$TMP_JSON_FILE"
  rm "$JSON_FILE"
  mv "$TMP_JSON_FILE" "$JSON_FILE"

  # Perform API call
  log_info "Uploading scene files to API..."
  local RESPONSE
  RESPONSE=$(curl --silent --write-out "\n%{http_code}" --location "$BASE_URL" \
    --header "Authorization: APIKEY:DEVELOPER $C3D_DEVELOPER_API_KEY" \
    --form "scene.bin=@$BIN_FILE" \
    --form "scene.gltf=@$GLTF_FILE" \
    --form "screenshot.png=@$PNG_FILE" \
    --form "settings.json=@$JSON_FILE")

  # Separate body and status code
  local HTTP_BODY=$(echo "$RESPONSE" | sed '$d')
  local HTTP_STATUS=$(echo "$RESPONSE" | tail -n1)

  if [[ "$HTTP_STATUS" -ge 200 && "$HTTP_STATUS" -lt 300 ]]; then
    log_info "Upload successful. Server response:"
    echo "$HTTP_BODY"
  else
    log_error "Upload failed with status $HTTP_STATUS"
    echo "$HTTP_BODY"
    exit 1
  fi

  log_info "Script complete."

  log_info "You can now upload your dynamic objects using the upload-object.sh script."
  log_info "You'll need the scene ID from the upload response."
  log_info "Example: ./upload-object.sh --scene_id <scene_id> --object_filename <object_filename> --object_dir <object_directory>"
  log_info "For more details, refer to the README file."
}

# Run main
main "$@"
