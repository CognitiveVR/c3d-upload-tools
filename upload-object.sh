#!/bin/bash

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
        echo "Usage: $0 [--scene_id <scene_id>] --object_filename <object_filename> --object_dir <object_directory> [--object_id <object_id>] [--env <prod|dev>] [--verbose] [--dry_run]"
        echo "  --scene_id        Scene ID to upload object to (or set C3D_SCENE_ID environment variable)"
        echo "  --object_filename Object filename (without extension)"
        echo "  --object_dir      Path to directory containing object files"
        echo "  --object_id       Optional. Object ID (defaults to object_filename)"
        echo "  --env             Optional. Either 'prod' (default) or 'dev'"
        echo "  --verbose         Optional. Enables verbose output"
        echo "  --dry_run         Optional. Preview operations without executing them"
        echo
        echo "Environment Variables:"
        echo "  C3D_DEVELOPER_API_KEY   Your Cognitive3D developer API key"
        echo "  C3D_SCENE_ID            Default scene ID (avoids --scene_id parameter)"
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

  log_debug "Upload URL: $UPLOAD_URL"
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
  # Create or Overwrite Manifest File
  # -------------------------------
  local MANIFEST_FILE="${SCENE_ID}_object_manifest.json"
  log_info "Creating manifest file: $MANIFEST_FILE"

  cat > "$MANIFEST_FILE" <<EOF
{
  "objects": [
    {
      "id": "$OBJECT_ID",
      "mesh": "$OBJECT_FILENAME",
      "name": "$OBJECT_FILENAME",
      "scaleCustom": [
        1.0,
        1.0,
        1.0
      ],
      "initialPosition": [
        0.0,
        0.0,
        0.0
      ],
      "initialRotation": [
        0.0,
        0.0,
        0.0,
        1.0
      ]
    }
  ]
}
EOF

  log_debug "Manifest file created: $MANIFEST_FILE"

  log_info "Automatically uploading the manifest."
  ./upload-object-manifest.sh --scene_id "$SCENE_ID" --env "$ENVIRONMENT" --verbose

  # Calculate and log total execution time
  log_execution_time "$start_time" "Object upload process"
  log_info "Upload complete. Object ID: $OBJECT_ID"
}

# Run main
main "$@"