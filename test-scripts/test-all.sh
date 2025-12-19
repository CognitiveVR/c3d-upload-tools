#!/bin/bash

# usage: test-all.sh [scene_id] [env]
# This script runs a series of upload and list commands for a given scene and environment.

# Required: Run `../upload-scene.sh` script with no scene_id first; use that scene_id for this script.

# Source utilities to get load_env_file function
source ../upload-utils.sh

# Load environment variables from .env file if it exists
load_env_file

# get SCENE_ID from the first argument or environment variable
SCENE_ID="${1:-${C3D_SCENE_ID:-}}"

# get ENV from the second argument, environment variable, or default to prod
ENV="${2:-${C3D_DEFAULT_ENVIRONMENT:-prod}}"

# Validate SCENE_ID is provided
if [[ -z "$SCENE_ID" ]]; then
  echo "Error: Scene ID is required"
  echo "Usage: $0 [scene_id] [env]"
  echo "  scene_id: Scene ID UUID (or set C3D_SCENE_ID environment variable)"
  echo "  env:      Environment - 'prod' or 'dev' (or set C3D_DEFAULT_ENVIRONMENT)"
  echo
  echo "Environment Variables (.env file or exported):"
  echo "  C3D_SCENE_ID            Default scene ID"
  echo "  C3D_DEFAULT_ENVIRONMENT Default environment (prod/dev)"
  echo "  C3D_DEVELOPER_API_KEY   Your Cognitive3D developer API key"
  exit 1
fi

echo "Running tests for scene ID: $SCENE_ID in environment: $ENV"
if [[ -n "${C3D_SCENE_ID:-}" && "$SCENE_ID" == "$C3D_SCENE_ID" ]]; then
  echo "Using C3D_SCENE_ID from environment (.env file or exported variable)"
fi
if [[ -n "${C3D_DEFAULT_ENVIRONMENT:-}" && "$ENV" == "$C3D_DEFAULT_ENVIRONMENT" ]]; then
  echo "Using C3D_DEFAULT_ENVIRONMENT from environment: $ENV"
fi
## wait for enter key
read -p "Press Enter to continue..."

../upload-scene.sh --scene_id $SCENE_ID --scene_dir ../scene-test --env $ENV --verbose

# Set C3D_SCENE_ID for remaining commands to test environment variable fallback
export C3D_SCENE_ID="$SCENE_ID"

../upload-object.sh --object_filename cube --object_id cube --object_dir ../object-test --env $ENV --verbose

../upload-object-manifest.sh --env $ENV --verbose

../upload-object.sh --object_filename Lantern --object_id Lantern-PNG --object_dir ../lantern-test --env $ENV --verbose

../upload-object-manifest.sh --env $ENV --verbose

../upload-object.sh --object_filename Lantern --object_id Lantern-JPEG --object_dir ../lantern-test-jpg --env $ENV --verbose

../upload-object-manifest.sh --env $ENV --verbose

../list-objects.sh --env $ENV --verbose
