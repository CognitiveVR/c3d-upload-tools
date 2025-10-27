#!/bin/bash

# Source shared utilities
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/upload-utils.sh"

# Load environment variables from .env file (if present)
load_env_file

# Uncomment for debugging
# set -x

# Define script variables
SCRIPT_NAME="$(basename "$0")"

# Main function
main() {
  # Default values
  SCENE_DIRECTORY=""
  ENVIRONMENT="${C3D_DEFAULT_ENVIRONMENT:-prod}"
  SCENE_ID="${C3D_SCENE_ID:-}"
  VERBOSE=false
  DRY_RUN=false

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
      --dry_run)
        DRY_RUN=true
        shift
        ;;
      --help|-h)
        echo "Usage: $SCRIPT_NAME --scene_dir <scene_directory> [--env <prod|dev>] [--scene_id <scene_id>] [--verbose] [--dry_run]"
        echo "  --scene_dir   Path to folder containing: scene.bin, scene.gltf, settings.json, screenshot.png"
        echo "  --env         Optional. Either 'prod' (default) or 'dev'"
        echo "  --scene_id    Optional. Appended to API URL if present"
        echo "  --verbose     Optional. Enables verbose output"
        echo "  --dry_run     Optional. Preview operations without executing them"
        echo
        echo "Required Files:"
        echo "  - scene.bin, scene.gltf, settings.json"
        echo "  - screenshot.png (required, used as primary scene screenshot)"
        echo
        echo "Optional Additional Images:"
        echo "  Any other PNG, JPG, or JPEG files in the scene directory will also be uploaded"
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

  # Fix log_verbose to use consistent debug logging (keeping for compatibility)
  log_verbose() { 
    log_debug "$1"
  }
  
  # Start timing
  local start_time=$(date +%s)
  log_info "Starting scene upload process"

  # Validate required CLI parameter
  if [[ -z "$SCENE_DIRECTORY" ]]; then
    log_error "Missing required argument: --scene_dir"
    echo "Usage: $SCRIPT_NAME --scene_dir <scene_directory> [--env <prod|dev>] [--scene_id <scene_id>] [--verbose] [--dry_run]"
    exit 1
  fi

  validate_directory "$SCENE_DIRECTORY" "scene directory"
  validate_environment "$ENVIRONMENT"
  log_info "Using environment: $ENVIRONMENT"
  
  # Validate scene_id format if provided (UUID format)
  if [[ -n "$SCENE_ID" ]]; then
    validate_uuid_format "$SCENE_ID" "scene_id"
    log_info "Using scene ID: $SCENE_ID"
  fi
  
  log_debug "SCENE_DIRECTORY: $SCENE_DIRECTORY"

  # Check dependencies and validate API key
  check_dependencies
  validate_api_key
  log_info "SCENE_DIRECTORY is: $SCENE_DIRECTORY"

  # Determine API base URL
  local BASE_URL
  BASE_URL=$(get_api_base_url "$ENVIRONMENT" "scenes")
  if [[ -n "$SCENE_ID" ]]; then
    BASE_URL+="/$SCENE_ID"
  fi
  log_info "Using API base URL: $BASE_URL"

  # Prepare file paths
  local BIN_FILE="$SCENE_DIRECTORY/scene.bin"
  local GLTF_FILE="$SCENE_DIRECTORY/scene.gltf"
  local JSON_FILE="$SCENE_DIRECTORY/settings.json"

  # Validate required files (bin, gltf, json)
  for file in "$BIN_FILE" "$GLTF_FILE" "$JSON_FILE"; do
    validate_file "$file" 100  # 100MB limit
  done

  # Handle screenshot.png as a required special file
  local SCREENSHOT_FILE="$SCENE_DIRECTORY/screenshot.png"
  validate_file "$SCREENSHOT_FILE" 100  # 100MB limit

  # Collect additional image files (png, jpg, jpeg) for upload
  local IMAGE_FORMS=()
  local IMAGE_COUNT=0
  for IMAGE_FILE in "$SCENE_DIRECTORY"/*.png "$SCENE_DIRECTORY"/*.jpg "$SCENE_DIRECTORY"/*.jpeg; do
    # Skip if no matching files (bash glob expands literally if no match)
    [[ -f "$IMAGE_FILE" ]] || continue

    local IMAGE_NAME=$(basename "$IMAGE_FILE")

    # Skip screenshot.png as it's handled separately
    if [[ "$IMAGE_NAME" == "screenshot.png" ]]; then
      continue
    fi

    validate_file "$IMAGE_FILE" 100  # 100MB limit
    IMAGE_FORMS+=(--form "$IMAGE_NAME=@$IMAGE_FILE")
    log_debug "Adding additional image: $IMAGE_NAME"
    ((IMAGE_COUNT++))
  done

  if [[ $IMAGE_COUNT -gt 0 ]]; then
    log_info "Found $IMAGE_COUNT additional image file(s) to upload (plus required screenshot.png)"
  else
    log_info "Uploading screenshot.png (no additional images found)"
  fi

  # Read sdk-version.txt
  local SDK_VERSION_FILE="$SCRIPT_DIR/sdk-version.txt"
  if [[ ! -s "$SDK_VERSION_FILE" ]]; then
    log_error "sdk-version.txt is missing or empty at: $SDK_VERSION_FILE"
    exit 1
  fi
  local SDK_VERSION
  SDK_VERSION=$(cat "$SDK_VERSION_FILE")
  
  # Validate SDK version format (semantic versioning: x.y.z)
  validate_semantic_version "$SDK_VERSION" "SDK version"
  
  log_info "Read SDK version: $SDK_VERSION"

  # Update settings.json with new sdkVersion using jq
  local TMP_JSON_FILE="$SCENE_DIRECTORY/settings-updated.json"
  local BACKUP_JSON_FILE="$JSON_FILE.backup"
  
  log_info "Updating settings.json with SDK version: $SDK_VERSION"
  
  if [[ "$DRY_RUN" = true ]]; then
    log_info "DRY RUN - Would perform these file operations:"
    echo "  1. Create backup: cp '$JSON_FILE' '$BACKUP_JSON_FILE'"
    echo "  2. Update JSON: jq --arg sdk '$SDK_VERSION' '.sdkVersion = \$sdk' '$BACKUP_JSON_FILE' > '$TMP_JSON_FILE'"
    echo "  3. Replace file: mv '$TMP_JSON_FILE' '$JSON_FILE'"
    echo "  4. Clean backup: rm '$BACKUP_JSON_FILE'"
  else
    # Create backup of original settings.json
    if ! cp "$JSON_FILE" "$BACKUP_JSON_FILE"; then
      log_error "Failed to create backup of settings.json"
      exit 1
    fi
    log_debug "Created backup: $BACKUP_JSON_FILE"
    
    # Update settings.json with jq
    if ! jq --arg sdk "$SDK_VERSION" '.sdkVersion = $sdk' "$BACKUP_JSON_FILE" > "$TMP_JSON_FILE"; then
      log_error "Failed to update settings.json with jq"
      rm -f "$TMP_JSON_FILE"
      rm -f "$BACKUP_JSON_FILE"
      exit 1
    fi
    log_debug "Updated JSON written to temporary file"
    
    # Replace original file with updated version
    if ! mv "$TMP_JSON_FILE" "$JSON_FILE"; then
      log_error "Failed to replace settings.json with updated version"
      # Attempt rollback
      log_warn "Attempting to restore backup..."
      if mv "$BACKUP_JSON_FILE" "$JSON_FILE"; then
        log_info "Successfully restored backup"
      else
        log_error "Failed to restore backup - settings.json may be corrupted"
      fi
      exit 1
    fi
    log_debug "Successfully replaced settings.json with updated version"
    
    # Clean up backup file
    rm -f "$BACKUP_JSON_FILE"
    log_debug "Cleaned up backup file"
  fi

  # Perform API call
  if [[ "$DRY_RUN" = true ]]; then
    log_info "DRY RUN - Would execute this curl command:"
    echo "curl --silent --write-out \"\\n%{http_code}\" --location '$BASE_URL' \\"
    echo "  --header 'Authorization: APIKEY:DEVELOPER [REDACTED]' \\"
    echo "  --form 'scene.bin=@$BIN_FILE' \\"
    echo "  --form 'scene.gltf=@$GLTF_FILE' \\"
    echo "  --form 'screenshot.png=@$SCREENSHOT_FILE' \\"
    # Print additional image forms
    for form in "${IMAGE_FORMS[@]}"; do
      echo "  $form \\"
    done
    echo "  --form 'settings.json=@$JSON_FILE'"
    echo ""
    echo "Files that would be uploaded:"
    for file in "$BIN_FILE" "$GLTF_FILE" "$SCREENSHOT_FILE" "$JSON_FILE"; do
      if [[ -f "$file" ]]; then
        local file_size
        if command -v stat >/dev/null 2>&1; then
          file_size=$(stat -f%z "$file" 2>/dev/null || stat -c%s "$file" 2>/dev/null)
          echo "  - $(basename "$file"): $(($file_size / 1024))KB"
        else
          echo "  - $(basename "$file"): [size unknown]"
        fi
      fi
    done
    # Print additional image files (excluding screenshot.png which is already listed)
    for IMAGE_FILE in "$SCENE_DIRECTORY"/*.png "$SCENE_DIRECTORY"/*.jpg "$SCENE_DIRECTORY"/*.jpeg; do
      [[ -f "$IMAGE_FILE" ]] || continue
      local IMAGE_NAME=$(basename "$IMAGE_FILE")
      # Skip screenshot.png as it's already listed
      [[ "$IMAGE_NAME" == "screenshot.png" ]] && continue

      local file_size
      if command -v stat >/dev/null 2>&1; then
        file_size=$(stat -f%z "$IMAGE_FILE" 2>/dev/null || stat -c%s "$IMAGE_FILE" 2>/dev/null)
        echo "  - $IMAGE_NAME: $(($file_size / 1024))KB"
      else
        echo "  - $IMAGE_NAME: [size unknown]"
      fi
    done
    
    # Calculate and log total execution time
    local end_time=$(date +%s)
    local total_duration=$((end_time - start_time))
    log_info "DRY RUN completed in ${total_duration} seconds"
    log_info "Re-run without --dry_run to perform actual upload"
    exit 0
  else
    log_info "Uploading scene files to API..."
    log_debug "Upload URL: $BASE_URL"
    log_debug "Files to upload: scene.bin, scene.gltf, images (png/jpg/jpeg), settings.json"

    # Build curl command array
    local CURL_CMD=(curl --silent --write-out "\n%{http_code}" --location "$BASE_URL" \
      --header "Authorization: APIKEY:DEVELOPER ${C3D_DEVELOPER_API_KEY}" \
      --form "scene.bin=@$BIN_FILE" \
      --form "scene.gltf=@$GLTF_FILE" \
      --form "screenshot.png=@$SCREENSHOT_FILE")

    # Add additional image forms (if any)
    if [[ ${#IMAGE_FORMS[@]} -gt 0 ]]; then
      CURL_CMD+=("${IMAGE_FORMS[@]}")
    fi

    # Add settings.json
    CURL_CMD+=(--form "settings.json=@$JSON_FILE")

    local upload_start_time=$(date +%s)
    local RESPONSE
    RESPONSE=$("${CURL_CMD[@]}")
    local upload_end_time=$(date +%s)
    local upload_duration=$((upload_end_time - upload_start_time))

    # Separate body and status code
    parse_http_response "$RESPONSE"

    log_info "Upload completed in ${upload_duration} seconds (HTTP $HTTP_STATUS)"

    if [[ "$HTTP_STATUS" -ge 200 && "$HTTP_STATUS" -lt 300 ]]; then
      process_json_response "$HTTP_BODY" "Upload"
    else
      handle_http_error "$HTTP_STATUS" "$HTTP_BODY" "Upload"
    fi

    # Calculate and log total execution time
    log_execution_time "$start_time" "Script"
  fi

  log_info "You can now upload your dynamic objects using the upload-object.sh script."
  log_info "You'll need the scene ID from the upload response."
  log_info "Example: ./upload-object.sh --scene_id <scene_id> --object_filename <object_filename> --object_dir <object_directory>"
  log_info "For more details, refer to the README file."
}

# Run main
main "$@"
