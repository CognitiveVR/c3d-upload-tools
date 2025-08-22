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

# Logging helpers with timestamps
log_info()  { echo -e "${COLOR_INFO}[$(date '+%Y-%m-%d %H:%M:%S')] [INFO] $1${COLOR_RESET}"; }
log_warn()  { echo -e "${COLOR_WARN}[$(date '+%Y-%m-%d %H:%M:%S')] [WARN] $1${COLOR_RESET}"; }
log_error() { echo -e "${COLOR_ERROR}[$(date '+%Y-%m-%d %H:%M:%S')] [ERROR] $1${COLOR_RESET}"; }
log_debug() { if [ "${VERBOSE:-false}" = true ]; then echo -e "${COLOR_DEBUG}[$(date '+%Y-%m-%d %H:%M:%S')] [DEBUG] $1${COLOR_RESET}"; fi; }

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
        echo "  --scene_dir   Path to folder containing 4 files: scene.bin, scene.gltf, screenshot.png, settings.json"
        echo "  --env         Optional. Either 'prod' (default) or 'dev'"
        echo "  --scene_id    Optional. Appended to API URL if present"
        echo "  --verbose     Optional. Enables verbose output"
        echo "  --dry_run     Optional. Preview operations without executing them"
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

  if [[ ! -d "$SCENE_DIRECTORY" ]]; then
    log_error "The specified scene directory does not exist: $SCENE_DIRECTORY"
    exit 1
  fi

  if [[ "$ENVIRONMENT" != "prod" && "$ENVIRONMENT" != "dev" ]]; then
    log_error "Invalid environment: $ENVIRONMENT. Must be 'prod' or 'dev'."
    exit 1
  fi

  log_info "Using environment: $ENVIRONMENT"
  
  # Validate scene_id format if provided (UUID format)
  if [[ -n "$SCENE_ID" ]]; then
    if [[ ! "$SCENE_ID" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ ]]; then
      log_error "Invalid scene_id format: $SCENE_ID"
      log_error "Expected UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
      exit 1
    fi
    log_info "Using scene ID: $SCENE_ID"
  fi
  
  log_debug "SCENE_DIRECTORY: $SCENE_DIRECTORY"

  # Import environment variable
  if [[ -z "${C3D_DEVELOPER_API_KEY:-}" ]]; then
    log_error "C3D_DEVELOPER_API_KEY is not set. Please set it with: export C3D_DEVELOPER_API_KEY=your_api_key"
    exit 1
  fi

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

  # Validate file existence and sizes
  local MAX_FILE_SIZE=$((100 * 1024 * 1024))  # 100MB limit
  
  for file in "$BIN_FILE" "$GLTF_FILE" "$PNG_FILE" "$JSON_FILE"; do
    if [[ ! -f "$file" ]]; then
      log_error "Required file missing: $file"
      exit 1
    fi
    
    # Check file size
    local file_size
    if command -v stat >/dev/null 2>&1; then
      # Try BSD stat first (macOS), then GNU stat (Linux)
      file_size=$(stat -f%z "$file" 2>/dev/null || stat -c%s "$file" 2>/dev/null)
      if [[ -n "$file_size" && $file_size -gt $MAX_FILE_SIZE ]]; then
        log_error "File too large (>100MB): $file ($(($file_size / 1024 / 1024))MB)"
        exit 1
      fi
      log_debug "File size OK: $(basename "$file") ($(($file_size / 1024))KB)"
    else
      log_warn "Cannot check file sizes - 'stat' command not available"
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
  
  # Validate SDK version format (semantic versioning: x.y.z)
  if [[ ! "$SDK_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    log_error "Invalid SDK version format: $SDK_VERSION"
    log_error "Expected semantic versioning format: x.y.z (e.g., 1.2.3)"
    exit 1
  fi
  
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
    echo "  --form 'screenshot.png=@$PNG_FILE' \\"
    echo "  --form 'settings.json=@$JSON_FILE'"
    echo ""
    echo "Files that would be uploaded:"
    for file in "$BIN_FILE" "$GLTF_FILE" "$PNG_FILE" "$JSON_FILE"; do
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
    
    # Calculate and log total execution time
    local end_time=$(date +%s)
    local total_duration=$((end_time - start_time))
    log_info "DRY RUN completed in ${total_duration} seconds"
    log_info "Re-run without --dry_run to perform actual upload"
    exit 0
  else
    log_info "Uploading scene files to API..."
    log_debug "Upload URL: $BASE_URL"
    log_debug "Files to upload: scene.bin, scene.gltf, screenshot.png, settings.json"
    
    local upload_start_time=$(date +%s)
    local RESPONSE
    RESPONSE=$(curl --silent --write-out "\n%{http_code}" --location "$BASE_URL" \
      --header "Authorization: APIKEY:DEVELOPER ${C3D_DEVELOPER_API_KEY}" \
      --form "scene.bin=@$BIN_FILE" \
      --form "scene.gltf=@$GLTF_FILE" \
      --form "screenshot.png=@$PNG_FILE" \
      --form "settings.json=@$JSON_FILE")
    local upload_end_time=$(date +%s)
    local upload_duration=$((upload_end_time - upload_start_time))

    # Separate body and status code
    local HTTP_BODY=$(echo "$RESPONSE" | sed '$d')
    local HTTP_STATUS=$(echo "$RESPONSE" | tail -n1)

    log_info "Upload completed in ${upload_duration} seconds (HTTP $HTTP_STATUS)"

    if [[ "$HTTP_STATUS" -ge 200 && "$HTTP_STATUS" -lt 300 ]]; then
      log_info "Upload successful. Server response (sanitized):"
      # Try to format JSON response, fall back to raw output
      if echo "$HTTP_BODY" | jq '.' >/dev/null 2>&1; then
        echo "$HTTP_BODY" | jq '.'
      else
        echo "$HTTP_BODY"
      fi
    else
      log_error "Upload failed with status $HTTP_STATUS"
      
      # Handle specific error cases with actionable guidance
      if [[ "$HTTP_STATUS" = "401" ]]; then
        if echo "$HTTP_BODY" | grep -i "key expired" >/dev/null 2>&1; then
          log_error "Your developer API key has expired."
          echo ""
          log_warn "To fix this issue:"
          echo "  1. Log into the Cognitive3D dashboard"
          echo "  2. Go to Settings (gear icon) → 'Manage developer key'"
          echo "  3. Generate a new developer API key"
          echo "  4. Update your environment variable: export C3D_DEVELOPER_API_KEY=\"your_new_key\""
          echo ""
          log_info "Once you have a new key, re-run this command to upload your scene."
        else
          log_error "Authentication failed. Please check your developer API key."
          echo ""
          log_warn "Verify your API key is correct:"
          echo "  1. Check the Cognitive3D dashboard: Settings → 'Manage developer key'"
          echo "  2. Ensure you're using the correct environment (--env prod or --env dev)"
          echo "  3. Update your key: export C3D_DEVELOPER_API_KEY=\"your_correct_key\""
        fi
      elif [[ "$HTTP_STATUS" = "403" ]]; then
        log_error "Access forbidden. Your API key may not have permission for this operation."
        echo ""
        log_warn "Contact support if you believe this is an error."
      elif [[ "$HTTP_STATUS" = "404" ]]; then
        log_error "Scene not found. The scene_id may be incorrect or the scene may not exist."
        echo ""
        log_warn "Check your scene_id and ensure the scene exists in the dashboard."
      else
        log_error "Server response:"
        echo "$HTTP_BODY"
      fi
      
      exit 1
    fi

    # Calculate and log total execution time
    local end_time=$(date +%s)
    local total_duration=$((end_time - start_time))
    log_info "Script completed successfully in ${total_duration} seconds"
  fi

  log_info "You can now upload your dynamic objects using the upload-object.sh script."
  log_info "You'll need the scene ID from the upload response."
  log_info "Example: ./upload-object.sh --scene_id <scene_id> --object_filename <object_filename> --object_dir <object_directory>"
  log_info "For more details, refer to the README file."
}

# Run main
main "$@"
