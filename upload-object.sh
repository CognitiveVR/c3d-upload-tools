#!/bin/bash

# upload-object.sh - Upload dynamic 3D objects to Cognitive3D platform
#
# API INTERACTION FLOW (aligned with Unity SDK):
#
# 1. Pre-upload Version Check:
#    GET /v0/scenes/{sceneId}
#    - Returns: JSON with versionNumber and versionId
#    - Unity Reference: EditorCore.cs:453-578 (RefreshSceneVersion)
#
# 2. Object Upload:
#    POST /v0/objects/{sceneId}/{objectId}?version={versionNumber}
#    - Content-Type: multipart/form-data
#    - Unity Reference: ExportUtility.cs:2276-2456 (UploadDynamicObjects)
#
# 3. Manifest Accumulation:
#    - Merges object into {sceneId}_object_manifest.json
#    - Unity Reference: EditorCore.cs:3036-3099 (manifest generation)
#
# 4. Manifest Upload (separate script):
#    POST /v0/objects/{sceneId}?version={versionNumber}
#    - Unity Reference: EditorCore.cs:2991-3139 (UploadManifest)
#
# IMPORTANT: This implementation matches Unity SDK API patterns.
# Objects are uploaded one at a time (user-specified directory).
# Manifest is accumulated and uploaded separately after all objects.

# Source shared utilities
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/upload-utils.sh"

# Load environment variables from .env file (if present)
load_env_file

# Main function
main() {
  # Default values
  ENVIRONMENT="${C3D_DEFAULT_ENVIRONMENT:-prod}"
  OBJECT_ID=""
  VERBOSE=false
  DRY_RUN=false
  SCENE_ID="${C3D_SCENE_ID:-}"
  OBJECT_FILENAME=""
  OBJECT_DIRECTORY=""

  # Parse named arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --scene_id)
        SCENE_ID="$2"
        shift 2
        ;;
      --object_filename)
        OBJECT_FILENAME="$2"
        shift 2
        ;;
      --object_id)
        OBJECT_ID="$2"
        shift 2
        ;;
      --env)
        ENVIRONMENT="$2"
        shift 2
        ;;
      --object_dir)
        OBJECT_DIRECTORY="$2"
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
        cat <<'EOF'
Usage: upload-object.sh [--scene_id <scene_id>] --object_filename <object_filename> --object_dir <object_directory> [OPTIONS]

Required:
  --scene_id <id>            Scene UUID (can also use C3D_SCENE_ID env var)
  --object_filename <name>   Object filename without extension
  --object_dir <path>        Directory containing object files

Optional:
  --object_id <id>           Custom object ID (defaults to filename)
  --env <prod|dev>           Environment (default: prod)
  --verbose                  Enable verbose logging
  --dry_run                  Preview operations without executing

Required Files in Object Directory:
  - <filename>.gltf          GLTF scene definition
  - <filename>.bin           Binary scene data
  - cvr_object_thumbnail.png Object thumbnail (required)
  - *.png, *.jpg, *.jpeg     Additional textures (optional)

Workflow:
  1. Upload objects: ./upload-object.sh --scene_id <id> --object_filename obj1 --object_dir dir1
  2. Upload more:    ./upload-object.sh --scene_id <id> --object_filename obj2 --object_dir dir2
  3. Upload manifest: ./upload-object-manifest.sh --scene_id <id> --env prod

Note: Manifest is accumulated locally but NOT uploaded automatically.
      After uploading all objects, run upload-object-manifest.sh to upload the manifest.

Examples:
  export C3D_SCENE_ID="76653a38-71a1-423a-a1b1-2fe6676033d6"
  export C3D_DEVELOPER_API_KEY="your_api_key_here"

  ./upload-object.sh --object_filename cube --object_dir ./objects
  ./upload-object.sh --object_filename lamp --object_dir ./objects
  ./upload-object-manifest.sh --env prod

Environment Variables:
  C3D_DEVELOPER_API_KEY   Your Cognitive3D developer API key
  C3D_SCENE_ID            Default scene ID (avoids --scene_id parameter)
EOF
        exit 0
        ;;
      *)
        log_error "Unknown argument: $1"
        exit 1
        ;;
    esac
  done

  # Start timing
  local start_time=$(date +%s)
  log_info "Starting object upload process"

  # Use environment variable fallback for scene_id
  if [[ -z "$SCENE_ID" ]]; then
    SCENE_ID="${C3D_SCENE_ID:-}"
    if [[ -n "$SCENE_ID" ]]; then
      log_debug "Using C3D_SCENE_ID from environment: $SCENE_ID"
    fi
  fi
  
  # Validate required arguments
  if [[ -z "$SCENE_ID" ]]; then
    log_error "Missing required argument: --scene_id (not provided as parameter or C3D_SCENE_ID environment variable)"
    echo "Usage: $0 [--scene_id <scene_id>] --object_filename <object_filename> --object_dir <object_directory> [--object_id <object_id>] [--env <prod|dev>] [--verbose] [--dry_run]"
    echo "       Set C3D_SCENE_ID environment variable to avoid --scene_id parameter"
    exit 1
  fi

  if [[ -z "$OBJECT_FILENAME" ]]; then
    log_error "Missing required argument: --object_filename"
    echo "Usage: $0 --scene_id <scene_id> --object_filename <object_filename> --object_dir <object_directory> [--object_id <object_id>] [--env <prod|dev>] [--verbose] [--dry_run]"
    exit 1
  fi

  if [[ -z "$OBJECT_DIRECTORY" ]]; then
    log_error "Missing required argument: --object_dir"
    echo "Usage: $0 --scene_id <scene_id> --object_filename <object_filename> --object_dir <object_directory> [--object_id <object_id>] [--env <prod|dev>] [--verbose] [--dry_run]"
    exit 1
  fi

  validate_directory "$OBJECT_DIRECTORY" "object directory"
  validate_environment "$ENVIRONMENT"
  validate_uuid_format "$SCENE_ID" "scene_id"

  # Check dependencies and validate API key
  check_dependencies
  validate_api_key

  log_info "Using environment: $ENVIRONMENT"

  # if object_id is not provided, it will be created from the object_filename
  if [[ -z "$OBJECT_ID" ]]; then
    OBJECT_ID=$(basename "$OBJECT_FILENAME")
    # OBJECT_ID="RANDOM_SOMETHING_1234"
    # make the OBJECT_ID a string as the current milliseconds timestamp to ensure uniqueness
    log_debug "Object ID not provided, using derived ID: $OBJECT_ID"
  fi

  # Log the parameters
  log_debug "Scene ID: $SCENE_ID"
  log_debug "Object Filename: $OBJECT_FILENAME"
  log_debug "Object Directory: $OBJECT_DIRECTORY"
  log_debug "Environment: $ENVIRONMENT"
  log_debug "Object ID: $OBJECT_ID"

  # Pre-upload version check (Unity SDK Reference: EditorCore.cs:453-578)
  # Get scene version before uploading objects to ensure version consistency
  log_info "Retrieving current scene version..."
  if get_scene_version "$SCENE_ID" "$ENVIRONMENT"; then
    log_info "Will upload to scene version: $SCENE_VERSION_NUMBER"

    # Validate version number is present
    if [[ -z "$SCENE_VERSION_NUMBER" ]]; then
      log_error "Failed to retrieve scene version number"
      log_error "Cannot upload object without version information"
      exit 1
    fi
  else
    log_error "Failed to retrieve scene version information"
    log_error "Object upload requires valid scene version"
    exit 1
  fi
  echo ""

  # Construct file paths
  local GLTF_FILE="$OBJECT_DIRECTORY/${OBJECT_FILENAME}.gltf"
  local BIN_FILE="$OBJECT_DIRECTORY/${OBJECT_FILENAME}.bin"
  local THUMBNAIL_FILE="$OBJECT_DIRECTORY/cvr_object_thumbnail.png"

  # Verify required files exist
  validate_file "$GLTF_FILE"
  validate_file "$BIN_FILE"
  validate_file "$THUMBNAIL_FILE"

  # Collect texture files (png, jpg, jpeg - excluding thumbnail)
  local TEXTURE_FORMS=()
  for TEXTURE_FILE in "$OBJECT_DIRECTORY"/*.png "$OBJECT_DIRECTORY"/*.jpg "$OBJECT_DIRECTORY"/*.jpeg; do
    # Skip if no matching files (bash glob expands literally if no match)
    [[ -f "$TEXTURE_FILE" ]] || continue

    if [[ "$TEXTURE_FILE" != "$THUMBNAIL_FILE" ]]; then
      local TEXTURE_NAME=$(basename "$TEXTURE_FILE")
      TEXTURE_FORMS+=(--form "$TEXTURE_NAME=@$TEXTURE_FILE")
      log_debug "Adding texture: $TEXTURE_NAME"
    fi
  done

  # Construct upload URL
  local API_BASE_URL
  API_BASE_URL=$(get_api_base_url "$ENVIRONMENT" "objects")
  local UPLOAD_URL="$API_BASE_URL/$SCENE_ID"
  if [[ -n "$OBJECT_ID" ]]; then
    UPLOAD_URL+="/$OBJECT_ID"
  fi

  # Add version parameter (Unity SDK Reference: CognitiveStatics.cs:52-55)
  # Format: /v0/objects/{sceneId}/{objectId}?version={versionNumber}
  UPLOAD_URL+="?version=${SCENE_VERSION_NUMBER}"

  log_debug "Upload URL with version: $UPLOAD_URL"
  log_debug "Using API key from environment variable"

  # Build curl command array
  local CURL_CMD=(curl --silent --write-out "\n%{http_code}" --location --globoff "$UPLOAD_URL" \
    --header "Authorization: APIKEY:DEVELOPER $C3D_DEVELOPER_API_KEY" \
    --form "cvr_object_thumbnail.png=@$THUMBNAIL_FILE" \
    --form "${OBJECT_FILENAME}.bin=@$BIN_FILE" \
    --form "${OBJECT_FILENAME}.gltf=@$GLTF_FILE")

  # Add texture .png files to curl command
  if [[ ${#TEXTURE_FORMS[@]} -gt 0 ]]; then
    CURL_CMD+=("${TEXTURE_FORMS[@]}")
  fi

  # Show and optionally skip execution
  if [[ "$DRY_RUN" = true ]]; then
    log_info "DRY RUN - Would execute this curl command:"
    # Print command with redacted API key for security
    printf '%q ' "${CURL_CMD[@]}"
    echo
    log_info "DRY RUN completed"  
    log_info "Re-run without --dry_run to perform actual upload"
    exit 0
  fi

  log_info "Uploading object files to API..."
  log_debug "Upload URL: $UPLOAD_URL"
  log_debug "Files to upload: ${OBJECT_FILENAME}.bin, ${OBJECT_FILENAME}.gltf, cvr_object_thumbnail.png, textures (png/jpg/jpeg)"

  local upload_start_time=$(date +%s)
  local RESPONSE
  RESPONSE=$("${CURL_CMD[@]}")
  local upload_end_time=$(date +%s)
  local upload_duration=$((upload_end_time - upload_start_time))

  # Separate body and status code
  parse_http_response "$RESPONSE"

  log_info "Upload completed in ${upload_duration} seconds (HTTP $HTTP_STATUS)"

  if [[ "$HTTP_STATUS" -ge 200 && "$HTTP_STATUS" -lt 300 ]]; then
    process_json_response "$HTTP_BODY" "Object upload"
  else
    handle_http_error "$HTTP_STATUS" "$HTTP_BODY" "Object upload"
  fi

  # -------------------------------
  # Generate/Update Object Manifest (Unity SDK Reference: EditorCore.cs:3036-3099)
  # -------------------------------
  local MANIFEST_FILE="${SCENE_ID}_object_manifest.json"

  log_info "Updating object manifest: $MANIFEST_FILE"

  # Create new object entry with Unity SDK format (4 decimal places)
  local NEW_OBJECT=$(cat <<'EOF'
{
  "id": "OBJECT_ID_PLACEHOLDER",
  "mesh": "MESH_PLACEHOLDER",
  "name": "NAME_PLACEHOLDER",
  "scaleCustom": [1.0000, 1.0000, 1.0000],
  "initialPosition": [0.0000, 0.0000, 0.0000],
  "initialRotation": [0.0000, 0.0000, 0.0000, 1.0000]
}
EOF
  )

  # Replace placeholders
  NEW_OBJECT=$(echo "$NEW_OBJECT" | sed "s/OBJECT_ID_PLACEHOLDER/$OBJECT_ID/g" | sed "s/MESH_PLACEHOLDER/$OBJECT_FILENAME/g" | sed "s/NAME_PLACEHOLDER/$OBJECT_FILENAME/g")

  # Merge with existing manifest or create new one
  if [[ -f "$MANIFEST_FILE" ]]; then
    log_debug "Manifest file exists, merging with existing objects"

    # Check if object ID already exists in manifest
    if jq -e ".objects[] | select(.id == \"$OBJECT_ID\")" "$MANIFEST_FILE" >/dev/null 2>&1; then
      log_warn "Object ID '$OBJECT_ID' already exists in manifest, updating entry"
      # Update existing object entry
      jq --argjson obj "$NEW_OBJECT" \
        '(.objects[] | select(.id == $obj.id)) = $obj' \
        "$MANIFEST_FILE" > "${MANIFEST_FILE}.tmp" && mv "${MANIFEST_FILE}.tmp" "$MANIFEST_FILE"
    else
      log_debug "Adding new object to manifest"
      # Append new object to objects array
      jq --argjson obj "$NEW_OBJECT" \
        '.objects += [$obj]' \
        "$MANIFEST_FILE" > "${MANIFEST_FILE}.tmp" && mv "${MANIFEST_FILE}.tmp" "$MANIFEST_FILE"
    fi
  else
    log_debug "Creating new manifest file"
    # Create new manifest with single object
    echo "{\"objects\": [$NEW_OBJECT]}" | jq '.' > "$MANIFEST_FILE"
  fi

  log_info "Manifest updated: $MANIFEST_FILE"
  log_debug "Current manifest contains $(jq '.objects | length' "$MANIFEST_FILE") object(s)"

  # Manifest has been updated but NOT uploaded yet
  # Unity SDK uploads manifest AFTER all objects are uploaded (not after each one)
  log_info ""
  log_info "💡 TIP: Manifest file updated but NOT uploaded yet"
  log_info "   After uploading all objects, run:"
  log_info "   ./upload-object-manifest.sh --scene_id \"$SCENE_ID\" --env \"$ENVIRONMENT\""
  echo ""

  # Calculate and log total execution time
  log_execution_time "$start_time" "Object upload process"
  log_info "Upload complete. Object ID: $OBJECT_ID"
}

# Run main
main "$@"