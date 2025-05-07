#!/bin/bash

# Default values
environment="prod"
object_id=""

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
    *)
      echo "Unknown argument: $1"
      exit 1
      ;;
  esac
done

# Validate required arguments
if [[ -z "$scene_id" ]]; then
  echo "Error: --scene_id is required"
  exit 1
fi

if [[ -z "$object_filename" ]]; then
  echo "Error: --object_filename is required"
  exit 1
fi

if [[ -z "$OBJECT_DIRECTORY" ]]; then
  echo "Error: --object_dir is required"
  exit 1
fi

if [[ ! -d "$OBJECT_DIRECTORY" ]]; then
  echo "Error: Directory not found: $OBJECT_DIRECTORY"
  exit 1
fi

if [[ -z "$C3D_DEVELOPER_API_KEY" ]]; then
  echo "Error: Environment variable C3D_DEVELOPER_API_KEY is not set"
  exit 1
fi

# Construct file paths
gltf_file="$OBJECT_DIRECTORY/${object_filename}.gltf"
bin_file="$OBJECT_DIRECTORY/${object_filename}.bin"
thumbnail_file="$OBJECT_DIRECTORY/cvr_object_thumbnail.png"  # May be optional

# Verify files exist
if [[ ! -f "$gltf_file" ]]; then
  echo "Error: File not found: $gltf_file"
  exit 1
fi
if [[ ! -f "$bin_file" ]]; then
  echo "Error: File not found: $bin_file"
  exit 1
fi
if [[ ! -f "$thumbnail_file" ]]; then
  echo "Warning: Thumbnail file not found: $thumbnail_file"
fi

# Set API base URL based on environment
if [[ "$environment" == "prod" ]]; then
  api_base_url="https://data.cognitive3d.com/v0/objects"
elif [[ "$environment" == "dev" ]]; then
  api_base_url="https://data.c3ddev.com/v0/objects"
else
  echo "Error: Unknown environment '$environment'. Use 'prod' or 'dev'."
  exit 1
fi

# Construct upload URL
upload_url="$api_base_url/$scene_id"
if [[ -n "$object_id" ]]; then
  upload_url+="/$object_id"
fi

# Upload using curl in Postman-style format
echo "Uploading to $upload_url..."
curl --location --globoff "$upload_url" \
  --header "Authorization: APIKEY:DEVELOPER $C3D_DEVELOPER_API_KEY" \
  --form "cvr_object_thumbnail.png=@$thumbnail_file" \
  --form "${object_filename}.bin=@$bin_file" \
  --form "${object_filename}.gltf=@$gltf_file"

echo "Upload complete."
