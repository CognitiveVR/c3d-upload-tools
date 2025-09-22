#!/bin/bash

# upload-object-manifest.sh
# Upload a JSON object manifest to the Cognitive3D platform

# Source shared utilities
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/upload-utils.sh"

# Load environment variables from .env file (if present)
load_env_file

# Main function
main() {
  # Default values
  SCENE_ID="${C3D_SCENE_ID:-}"
  ENVIRONMENT="${C3D_DEFAULT_ENVIRONMENT:-prod}"
  VERBOSE=false
  DRY_RUN=false

  # Start timing
  local start_time=$(date +%s)
  log_info "Starting object manifest upload process"

  # Parse named arguments
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --scene_id)
        SCENE_ID="$2"
        shift 2
        ;;
      --env)
        ENVIRONMENT="$2"
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
        echo "Usage: $0 [--scene_id <scene_id>] --env <prod|dev> [--verbose] [--dry_run]"
        echo "  --scene_id   Scene ID (or set C3D_SCENE_ID environment variable)"
        echo "  --env        Either 'prod' (default) or 'dev'"
        echo "  --verbose    Optional. Enables verbose output"
        echo "  --dry_run    Optional. Preview operations without executing them"
        echo
        echo "Environment Variables:"
        echo "  C3D_DEVELOPER_API_KEY   Your Cognitive3D developer API key"
        echo "  C3D_SCENE_ID            Default scene ID (avoids --scene_id parameter)"
        exit 0
        ;;
      *)
        log_error "Unknown argument: $1"
        echo "Usage: $0 --scene_id <scene_id> --env <prod|dev> [--verbose] [--dry_run]"
        exit 1
        ;;
    esac
  done

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
    echo "Usage: $0 [--scene_id <scene_id>] --env <prod|dev> [--verbose] [--dry_run]"
    echo "       Set C3D_SCENE_ID environment variable to avoid --scene_id parameter"
    exit 1
  fi

  validate_uuid_format "$SCENE_ID" "scene_id"
  validate_environment "$ENVIRONMENT"
  log_info "Using environment: $ENVIRONMENT"

  # Check dependencies and validate API key
  check_dependencies
  validate_api_key

  # Set API URL and JSON file
  local ENDPOINT
  ENDPOINT=$(get_api_base_url "$ENVIRONMENT" "objects")
  ENDPOINT+="/$SCENE_ID"
  
  local MANIFEST_FILE="${SCENE_ID}_object_manifest.json"
  validate_file "$MANIFEST_FILE"
  
  log_debug "Scene ID: $SCENE_ID"
  log_debug "Environment: $ENVIRONMENT"
  log_debug "Endpoint: $ENDPOINT"
  log_debug "Manifest File: $MANIFEST_FILE"

  # Execute POST Request
  if [[ "$DRY_RUN" = true ]]; then
    log_info "DRY RUN - Would execute this curl command:"
    echo "curl -s -w '\\n%{http_code}' -X POST '$ENDPOINT' \\"
    echo "  -H 'Content-Type: application/json' \\"
    echo "  -H 'Authorization: APIKEY:DEVELOPER [REDACTED]' \\"
    echo "  --data-binary '@$MANIFEST_FILE'"
    echo ""
    log_info "File that would be uploaded:"
    echo "  - $MANIFEST_FILE"
    log_info "DRY RUN completed"
    log_info "Re-run without --dry_run to perform actual upload"
    exit 0
  fi

  log_info "Uploading object manifest..."
  log_debug "Endpoint: $ENDPOINT"
  log_debug "Manifest file: $MANIFEST_FILE"
  
  local upload_start_time=$(date +%s)
  local RESPONSE
  RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$ENDPOINT" \
    -H "Content-Type: application/json" \
    -H "Authorization: APIKEY:DEVELOPER $C3D_DEVELOPER_API_KEY" \
    --data-binary "@$MANIFEST_FILE")
  local upload_end_time=$(date +%s)
  local upload_duration=$((upload_end_time - upload_start_time))

  # Parse response
  parse_http_response "$RESPONSE"
  
  log_info "Upload completed in ${upload_duration} seconds (HTTP $HTTP_STATUS)"

  if [[ "$HTTP_STATUS" -ge 200 && "$HTTP_STATUS" -lt 300 ]]; then
    process_json_response "$HTTP_BODY" "Object manifest upload"
  else
    handle_http_error "$HTTP_STATUS" "$HTTP_BODY" "Object manifest upload"
  fi
  
  # Calculate and log total execution time
  log_execution_time "$start_time" "Object manifest upload process"
}

# Run main
main "$@"
