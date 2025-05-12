#!/bin/bash

# Default values
environment="prod"
object_id=""
verbose=false
dry_run=false

# ANSI color codes
COLOR_RESET="\033[0m"
COLOR_INFO="\033[1;34m"
COLOR_WARN="\033[1;33m"
COLOR_ERROR="\033[1;31m"
COLOR_DEBUG="\033[0;36m"

# --- Check Dependencies ---
if ! command -v jq >/dev/null 2>&1; then
  echo "Error: 'jq' is not installed. Please install it before running this script."
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "Error: 'curl' is not installed. Please install it before running this script."
  exit 1
fi

# Parse named arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --scene_id)
      scene_id="$2"
      shift 2
      ;;
    --object_filename)
      object_filename="$2"
      shift 2
      ;;
    --object_id)
      object_id="$2"
      shift 2
      ;;
    --env)
      environment="$2"
      shift 2
      ;;
    --object_dir)
      OBJECT_DIRECTORY="$2"
      shift 2
      ;;
    --verbose)
      verbose=true
      shift
      ;;
    --dry_run)
      dry_run=true
      shift
      ;;
    *)
      echo -e "${COLOR_ERROR}Unknown argument: $1${COLOR_RESET}"
      exit 1
      ;;
  esac
done

# Logging helpers
log_info()  { echo -e "${COLOR_INFO}[INFO] $1${COLOR_RESET}"; }
log_warn()  { echo -e "${COLOR_WARN}[WARN] $1${COLOR_RESET}"; }
log_error() { echo -e "${COLOR_ERROR}[ERROR] $1${COLOR_RESET}"; }
log_debug() { [ "$verbose" = true ] && echo -e "${COLOR_DEBUG}[DEBUG] $1${COLOR_RESET}"; }

# Validate required arguments
if [[ -z "$scene_id" ]]; then
  log_error "--scene_id is required"
  exit 1
fi

if [[ -z "$object_filename" ]]; then
  log_error "--object_filename is required"
  exit 1
fi

if [[ -z "$OBJECT_DIRECTORY" ]]; then
  log_error "--object_dir is required"
  exit 1
fi

if [[ ! -d "$OBJECT_DIRECTORY" ]]; then
  log_error "Directory not found: $OBJECT_DIRECTORY"
  exit 1
fi

if [[ -z "$C3D_DEVELOPER_API_KEY" ]]; then
  log_error "Environment variable C3D_DEVELOPER_API_KEY is not set"
  exit 1
fi

# if object_id is not provided, it will be created from the object_filename
if [[ -z "$object_id" ]]; then
  object_id=$(basename "$object_filename")
  log_debug "Object ID not provided, using derived ID: $object_id"
fi
# Log the parameters
log_debug "Scene ID: $scene_id"
log_debug "Object Filename: $object_filename"
log_debug "Object Directory: $OBJECT_DIRECTORY"
log_debug "Environment: $environment"
log_debug "Object ID: $object_id"

# Construct file paths
gltf_file="$OBJECT_DIRECTORY/${object_filename}.gltf"
bin_file="$OBJECT_DIRECTORY/${object_filename}.bin"
thumbnail_file="$OBJECT_DIRECTORY/cvr_object_thumbnail.png"

# Verify required files exist
if [[ ! -f "$gltf_file" ]]; then
  log_error "File not found: $gltf_file"
  exit 1
fi
if [[ ! -f "$bin_file" ]]; then
  log_error "File not found: $bin_file"
  exit 1
fi

# Collect texture .png files (excluding thumbnail)
texture_forms=()
for texture_file in "$OBJECT_DIRECTORY"/*.png; do
  if [[ "$texture_file" != "$thumbnail_file" ]]; then
    texture_name=$(basename "$texture_file")
    texture_forms+=(--form "$texture_name=@$texture_file")
    log_debug "Adding texture: $texture_name"
  fi
done

# Set API base URL based on environment
if [[ "$environment" == "prod" ]]; then
  api_base_url="https://data.cognitive3d.com/v0/objects"
elif [[ "$environment" == "dev" ]]; then
  api_base_url="https://data.c3ddev.com/v0/objects"
else
  log_error "Unknown environment '$environment'. Use 'prod' or 'dev'."
  exit 1
fi

# Construct upload URL
upload_url="$api_base_url/$scene_id"
if [[ -n "$object_id" ]]; then
  upload_url+="/$object_id"
fi

log_debug "Upload URL: $upload_url"
log_debug "Using API key from environment variable"

# Build curl command array
curl_cmd=(curl --silent --show-error --location --globoff "$upload_url" \
  --header "Authorization: APIKEY:DEVELOPER $C3D_DEVELOPER_API_KEY" \
  --form "cvr_object_thumbnail.png=@$thumbnail_file" \
  --form "${object_filename}.bin=@$bin_file" \
  --form "${object_filename}.gltf=@$gltf_file")

# Add texture .png files to curl command
curl_cmd+=("${texture_forms[@]}")

# Show and optionally skip execution
if [ "$verbose" = true ] || [ "$dry_run" = true ]; then
  echo -e "${COLOR_DEBUG}[DEBUG] Final curl command:${COLOR_RESET}"
  printf '%q ' "${curl_cmd[@]}"
  echo
fi

if [ "$dry_run" = true ]; then
  log_info "Dry run mode: upload skipped."
  exit 0
fi

log_info "Executing curl command..."
response=$("${curl_cmd[@]}" 2>&1)
status=$?

if [ $status -ne 0 ]; then
  log_error "Upload failed with status $status"
  log_error "Response: $response"
  exit $status
fi

log_info "Upload complete."
echo "$response"

# -------------------------------
# Create or Overwrite Manifest File
# -------------------------------
cat > "${scene_id}_object_manifest.json" <<EOF
{
  "objects": [
    {
      "id": "$object_id",
      "mesh": "$object_filename",
      "name": "$object_filename",
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

log_info "Manifest file created: ${scene_id}_object_manifest.json"
log_info "Upload complete. Object ID: $object_id"
