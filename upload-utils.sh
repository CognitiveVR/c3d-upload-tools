#!/bin/bash

# upload-utils.sh
# Shared utilities for Cognitive3D upload scripts
# Source this file with: source ./upload-utils.sh

# Exit immediately if a command exits with a non-zero status.
set -e

# Treat unset variables as an error.
set -u

# ANSI color codes
COLOR_RESET="\033[0m"
COLOR_INFO="\033[1;34m"
COLOR_WARN="\033[1;33m"
COLOR_ERROR="\033[1;31m"
COLOR_DEBUG="\033[0;36m"

# Logging helpers with timestamps (using upload-scene.sh pattern)
log_info()  { echo -e "${COLOR_INFO}[$(date '+%Y-%m-%d %H:%M:%S')] [INFO] $1${COLOR_RESET}"; }
log_warn()  { echo -e "${COLOR_WARN}[$(date '+%Y-%m-%d %H:%M:%S')] [WARN] $1${COLOR_RESET}"; }
log_error() { echo -e "${COLOR_ERROR}[$(date '+%Y-%m-%d %H:%M:%S')] [ERROR] $1${COLOR_RESET}"; }
log_debug() { if [ "${VERBOSE:-false}" = true ]; then echo -e "${COLOR_DEBUG}[$(date '+%Y-%m-%d %H:%M:%S')] [DEBUG] $1${COLOR_RESET}"; fi; }

# Backward compatibility alias
log_verbose() { 
  log_debug "$1"
}

# Load environment variables from .env file
load_env_file() {
  local env_file="${1:-.env}"
  
  if [[ -f "$env_file" ]]; then
    log_debug "Loading environment variables from $env_file"
    
    # Read .env file line by line
    while IFS= read -r line; do
      # Skip empty lines and comments
      [[ -z "$line" ]] && continue
      [[ "$line" =~ ^[[:space:]]*# ]] && continue
      
      # Skip lines without '='
      [[ ! "$line" =~ = ]] && continue
      
      # Extract key and value
      key="${line%%=*}"
      value="${line#*=}"
      
      # Trim whitespace
      key=$(echo "$key" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
      value=$(echo "$value" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
      
      # Skip if key is empty
      [[ -z "$key" ]] && continue
      
      # Validate key format (letters, numbers, underscore only)
      if [[ ! "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
        log_debug "Skipping invalid key format: $key"
        continue
      fi
      
      # Only set if not already set (existing env vars take precedence)
      if [[ -z $(printenv "$key" 2>/dev/null) ]]; then
        export "$key"="$value"
        log_debug "Set $key from .env file"
      else
        log_debug "Skipping $key (already set in environment)"
      fi
    done < "$env_file"
    
    log_debug "Finished loading environment variables from $env_file"
  else
    log_debug "No .env file found at $env_file"
  fi
}

# Check for required dependencies
check_dependencies() {
  local missing_deps=()
  
  if ! command -v jq >/dev/null 2>&1; then
    missing_deps+=("jq")
  fi
  
  if ! command -v curl >/dev/null 2>&1; then
    missing_deps+=("curl")
  fi
  
  if [ ${#missing_deps[@]} -gt 0 ]; then
    for dep in "${missing_deps[@]}"; do
      log_error "'$dep' is not installed. Please install it before running this script."
    done
    exit 1
  fi
}

# Validate API key environment variable
validate_api_key() {
  if [[ -z "${C3D_DEVELOPER_API_KEY:-}" ]]; then
    log_error "C3D_DEVELOPER_API_KEY is not set. Please set it with: export C3D_DEVELOPER_API_KEY=your_api_key"
    exit 1
  fi
  log_info "C3D_DEVELOPER_API_KEY has been set."
}

# Get API base URL based on environment
get_api_base_url() {
  local env="$1"
  local endpoint="${2:-scenes}"  # default to scenes, can be overridden
  
  case "$env" in
    prod)
      echo "https://data.cognitive3d.com/v0/${endpoint}"
      ;;
    dev)
      echo "https://data.c3ddev.com/v0/${endpoint}"
      ;;
    *)
      log_error "Unknown environment: $env"
      exit 1
      ;;
  esac
}

# Validate environment parameter
validate_environment() {
  local env="$1"
  if [[ "$env" != "prod" && "$env" != "dev" ]]; then
    log_error "Invalid environment: $env. Must be 'prod' or 'dev'."
    exit 1
  fi
}

# Validate UUID format
validate_uuid_format() {
  local uuid="$1"
  local field_name="${2:-UUID}"
  
  if [[ ! "$uuid" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ ]]; then
    log_error "Invalid $field_name format: $uuid"
    log_error "Expected UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    exit 1
  fi
}

# Parse HTTP response to separate body and status code
parse_http_response() {
  local response="$1"
  
  # Export as global variables for the calling script
  HTTP_BODY=$(echo "$response" | sed '$d')
  HTTP_STATUS=$(echo "$response" | tail -n1)
}

# Handle HTTP errors with specific guidance (using upload-scene.sh patterns)
handle_http_error() {
  local status="$1"
  local body="$2"
  local operation="${3:-Upload}"
  
  log_error "$operation failed with status $status"
  
  case "$status" in
    401)
      if echo "$body" | grep -i "key expired" >/dev/null 2>&1; then
        log_error "Your developer API key has expired."
        echo ""
        log_warn "To fix this issue:"
        echo "  1. Log into the Cognitive3D dashboard"
        echo "  2. Go to Settings (gear icon) → 'Manage developer key'"
        echo "  3. Generate a new developer API key"
        echo "  4. Update your environment variable: export C3D_DEVELOPER_API_KEY=\"your_new_key\""
        echo ""
        log_info "Once you have a new key, re-run this command."
      else
        log_error "Authentication failed. Please check your developer API key."
        echo ""
        log_warn "Verify your API key is correct:"
        echo "  1. Check the Cognitive3D dashboard: Settings → 'Manage developer key'"
        echo "  2. Ensure you're using the correct environment (--env prod or --env dev)"
        echo "  3. Update your key: export C3D_DEVELOPER_API_KEY=\"your_correct_key\""
      fi
      ;;
    403)
      log_error "Access forbidden. Your API key may not have permission for this operation."
      echo ""
      log_warn "Contact support if you believe this is an error."
      ;;
    404)
      log_error "Resource not found. The ID may be incorrect or the resource may not exist."
      echo ""
      log_warn "Check your IDs and ensure the resource exists in the dashboard."
      ;;
    *)
      log_error "Server response:"
      echo "$body"
      ;;
  esac
  
  exit 1
}

# Log execution time
log_execution_time() {
  local start_time="$1"
  local operation_name="${2:-Operation}"
  local end_time=$(date +%s)
  local duration=$((end_time - start_time))
  
  log_info "$operation_name completed in ${duration} seconds"
}

# Validate file existence and size
validate_file() {
  local file_path="$1"
  local max_size_mb="${2:-100}"  # Default 100MB limit
  local max_file_size=$((max_size_mb * 1024 * 1024))
  
  if [[ ! -f "$file_path" ]]; then
    log_error "Required file missing: $file_path"
    exit 1
  fi
  
  # Check file size
  if command -v stat >/dev/null 2>&1; then
    local file_size
    # Try BSD stat first (macOS), then GNU stat (Linux)
    file_size=$(stat -f%z "$file_path" 2>/dev/null || stat -c%s "$file_path" 2>/dev/null)
    if [[ -n "$file_size" && $file_size -gt $max_file_size ]]; then
      log_error "File too large (>${max_size_mb}MB): $file_path ($(($file_size / 1024 / 1024))MB)"
      exit 1
    fi
    log_debug "File size OK: $(basename "$file_path") ($(($file_size / 1024))KB)"
  else
    log_warn "Cannot check file sizes - 'stat' command not available"
  fi
}

# Validate directory existence
validate_directory() {
  local dir_path="$1"
  local description="${2:-directory}"
  
  if [[ ! -d "$dir_path" ]]; then
    log_error "The specified $description does not exist: $dir_path"
    exit 1
  fi
}

# Process JSON response with formatting
process_json_response() {
  local response_body="$1"
  local operation_name="${2:-Operation}"
  
  log_info "$operation_name successful. Server response (sanitized):"
  # Try to format JSON response, fall back to raw output
  if echo "$response_body" | jq '.' >/dev/null 2>&1; then
    echo "$response_body" | jq '.'
  else
    echo "$response_body"
  fi
}

# Validate semantic version format
validate_semantic_version() {
  local version="$1"
  local field_name="${2:-version}"
  
  if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    log_error "Invalid $field_name format: $version"
    log_error "Expected semantic versioning format: x.y.z (e.g., 1.2.3)"
    exit 1
  fi
}