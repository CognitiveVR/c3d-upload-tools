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

# Example function
echo_info() {
  echo "[INFO] $1"
}

echo_error() {
  echo "[ERROR] $1" >&2
}

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
      echo_error "Unknown environment: $env"
      exit 1
      ;;
  esac
}

# Main function
main() {
  echo_info "Running $SCRIPT_NAME from $SCRIPT_DIR"

  # Validate required CLI parameter
  if [[ $# -lt 1 ]]; then
    echo_error "Missing required argument: SCENE_DIRECTORY"
    echo "Usage: $SCRIPT_NAME <path_to_scene_directory> [dev|prod] [scene_id]"
    exit 1
  fi

  local SCENE_DIRECTORY="$1"

  if [[ ! -d "$SCENE_DIRECTORY" ]]; then
    echo_error "The specified scene directory does not exist: $SCENE_DIRECTORY"
    exit 1
  fi

  # Optional second argument: ENVIRONMENT (defaults to "prod")
  local ENVIRONMENT="prod"
  if [[ $# -ge 2 ]]; then
    if [[ "$2" != "prod" && "$2" != "dev" ]]; then
      echo_error "Invalid environment: $2. Must be 'prod' or 'dev'."
      exit 1
    fi
    ENVIRONMENT="$2"
  fi

  # Optional third argument: SCENE_ID
  local SCENE_ID=""
  if [[ $# -ge 3 ]]; then
    SCENE_ID="$3"
  fi

  echo_info "Using environment: $ENVIRONMENT"
  [[ -n "$SCENE_ID" ]] && echo_info "Using scene ID: $SCENE_ID"

  # Import environment variable
  if [[ -z "${C3D_DEVELOPER_API_KEY:-}" ]]; then
    echo_error "C3D_DEVELOPER_API_KEY is not set. Please set it with: export C3D_DEVELOPER_API_KEY=your_api_key"
    exit 1
  fi

  local C3D_DEVELOPER_API_KEY="$C3D_DEVELOPER_API_KEY"
  echo_info "C3D_DEVELOPER_API_KEY has been set."
  echo_info "SCENE_DIRECTORY is: $SCENE_DIRECTORY"

  # Determine API base URL
  local BASE_URL
  BASE_URL=$(get_api_base_url "$ENVIRONMENT")
  if [[ -n "$SCENE_ID" ]]; then
    BASE_URL+="/$SCENE_ID"
  fi
  echo_info "Using API base URL: $BASE_URL"

  # Prepare file paths
  local BIN_FILE="$SCENE_DIRECTORY/scene.bin"
  local GLTF_FILE="$SCENE_DIRECTORY/scene.gltf"
  local PNG_FILE="$SCENE_DIRECTORY/screenshot.png"
  local JSON_FILE="$SCENE_DIRECTORY/settings.json"

  # Validate file existence
  for file in "$BIN_FILE" "$GLTF_FILE" "$PNG_FILE" "$JSON_FILE"; do
    if [[ ! -f "$file" ]]; then
      echo_error "Required file missing: $file"
      exit 1
    fi
  done

  # Read sdk-version.txt
  local SDK_VERSION_FILE="$SCRIPT_DIR/sdk-version.txt"
  if [[ ! -f "$SDK_VERSION_FILE" ]]; then
    echo_error "sdk-version.txt not found in script directory."
    exit 1
  fi
  local SDK_VERSION
  SDK_VERSION=$(<"$SDK_VERSION_FILE")
  echo_info "Read SDK version: $SDK_VERSION"

  # Update settings.json with new sdkVersion
  local TMP_JSON_FILE="$SCENE_DIRECTORY/settings-updated.json"
  sed -E "s/(\"sdkVersion\"\s*:\s*\")([^"]+)(\")/\1$SDK_VERSION\3/" "$JSON_FILE" > "$TMP_JSON_FILE"
  rm "$JSON_FILE"
  mv "$TMP_JSON_FILE" "$JSON_FILE"

  # Perform API call
  echo_info "Uploading scene files to API..."
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
    echo_info "Upload successful. Server response:"
    echo "$HTTP_BODY"
  else
    echo_error "Upload failed with status $HTTP_STATUS"
    echo "$HTTP_BODY"
    exit 1
  fi

  echo_info "Script complete."
}

# Run main
main "$@"
