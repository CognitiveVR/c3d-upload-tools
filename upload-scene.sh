#!/bin/bash

# upload-scene.sh - Upload 3D scenes to Cognitive3D platform
#
# API INTERACTION FLOW (aligned with Unity SDK):
#
# 1. Pre-upload Version Check (for existing scenes):
#    GET /v0/scenes/{sceneId}
#    - Returns: JSON with versionNumber and versionId
#    - Unity Reference: EditorCore.cs:453-578 (RefreshSceneVersion)
#
# 2. Scene Upload (includes screenshot in multipart form):
#    POST /v0/scenes (new scene)
#    POST /v0/scenes/{sceneId} (update existing)
#    - Content-Type: multipart/form-data
#    - Includes: scene.bin, scene.gltf, screenshot.png, settings.json
#    - Unity Reference: ExportUtility.cs:367-550 (UploadDecimatedScene)
#
# 3. Success Response Formats:
#    - HTTP 201 (new scene): Plain text scene ID (e.g., "76653a38-71a1-423a-a1b1-2fe6676033d6")
#    - HTTP 200 (updated scene): Empty response body
#    - Unity Reference: ExportUtility.cs:495-550 (PostSceneUploadResponse)
#
# 4. Separate Screenshot Upload (after scene upload succeeds):
#    POST /v0/scenes/{sceneId}/screenshot?version={versionNumber}
#    - Content-Type: multipart/form-data
#    - Uploads screenshot.png via dedicated endpoint
#    - Unity Reference: EditorCore.cs:2357-2378 (UploadSceneThumbnail), UploadTools.cs:360
#
# 5. Error Response Handling:
#    - HTML error pages detected via content matching
#    - Specific guidance for 401 (expired key), 403 (forbidden), 404 (not found)
#    - Unity Reference: ExportUtility.cs:542-547 (HTML error detection)
#
# IMPORTANT: This implementation matches the Unity SDK's API interaction patterns
# to ensure consistency across different upload methods. Screenshots are uploaded
# both WITH the scene data AND separately via the dedicated screenshot endpoint.

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
        echo "  Any other PNG, JPG, JPEG, or WEBP files in the scene directory will also be uploaded"
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

  # Collect additional image files (png, jpg, jpeg, webp) for upload
  local IMAGE_FORMS=()
  local IMAGE_COUNT=0
  for IMAGE_FILE in "$SCENE_DIRECTORY"/*.png "$SCENE_DIRECTORY"/*.jpg "$SCENE_DIRECTORY"/*.jpeg "$SCENE_DIRECTORY"/*.webp; do
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

  # Pre-upload version check (Unity SDK Reference: EditorCore.cs:453-578)
  # If updating an existing scene, retrieve current version information first
  if [[ -n "$SCENE_ID" ]]; then
    log_info "Retrieving current scene version information..."
    if get_scene_version "$SCENE_ID" "$ENVIRONMENT"; then
      log_debug "Pre-upload version check completed successfully"
      # Version info is now available in SCENE_VERSION_NUMBER and SCENE_VERSION_ID globals
    else
      log_debug "Pre-upload version check skipped or failed (non-fatal) - continuing with upload"
    fi
    echo ""
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
    for IMAGE_FILE in "$SCENE_DIRECTORY"/*.png "$SCENE_DIRECTORY"/*.jpg "$SCENE_DIRECTORY"/*.jpeg "$SCENE_DIRECTORY"/*.webp; do
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
    log_debug "Files to upload: scene.bin, scene.gltf, images (png/jpg/jpeg/webp), settings.json"

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

    # Unity SDK Reference: ExportUtility.cs:495-550
    # Success codes: 200 (scene updated) or 201 (scene created)
    # Response formats:
    #   - HTTP 201: Plain text scene ID (new scene)
    #   - HTTP 200: Empty body (scene updated)
    if [[ "$HTTP_STATUS" -eq 200 ]] || [[ "$HTTP_STATUS" -eq 201 ]]; then
      if [[ "$HTTP_STATUS" -eq 201 ]]; then
        # New scene created - response body is plain text scene ID
        if [[ -n "$HTTP_BODY" ]]; then
          # Trim whitespace and quotes from scene ID
          SCENE_ID=$(echo "$HTTP_BODY" | tr -d '\n\r"' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
          log_info "✓ Scene created successfully!"
          log_info "Scene ID: $SCENE_ID"
          echo ""
          log_info "💡 TIP: Save this Scene ID for uploading objects:"
          log_info "   export C3D_SCENE_ID=\"$SCENE_ID\""
          log_info "   Or add it to your .env file: C3D_SCENE_ID=$SCENE_ID"
          # Output clean scene ID to stdout for programmatic parsing (no ANSI codes, no formatting)
          echo "$SCENE_ID"
        else
          log_warn "Scene created (HTTP 201) but no scene ID returned in response"
        fi
      elif [[ "$HTTP_STATUS" -eq 200 ]]; then
        # Scene updated - response body is typically empty
        log_info "✓ Scene updated successfully!"
        if [[ -n "$HTTP_BODY" ]]; then
          # Unexpected: got response body for update
          log_debug "Received unexpected response body:"
          if echo "$HTTP_BODY" | jq -e . >/dev/null 2>&1; then
            echo "$HTTP_BODY" | jq '.'
          else
            echo "$HTTP_BODY"
          fi
        fi
      fi

      # Separate screenshot upload (Unity SDK Reference: EditorCore.cs:2357-2378, UploadTools.cs:360)
      # Upload screenshot separately after scene upload succeeds
      echo ""
      log_info "Uploading screenshot via separate API call..."
      if upload_screenshot "$SCENE_ID" "$SCREENSHOT_FILE" "$ENVIRONMENT"; then
        log_debug "Screenshot upload completed successfully"
      else
        log_warn "Screenshot upload failed, but scene upload was successful"
        log_warn "You can manually retry screenshot upload later if needed"
      fi
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
