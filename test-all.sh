#!/bin/bash

# usage: test-all.sh <scene_id> <env>
# This script runs a series of upload and list commands for a given scene and environment.

# Required: Run `./upload-scene.sh` script with no scene_id first; use that scene_id for this script.

# get SCENE_ID from the first argument
SCENE_ID="$1"

# get ENV from the second argument
ENV="$2"

echo "Running tests for scene ID: $SCENE_ID in environment: $ENV"
## wait for enter key
read -p "Press Enter to continue..."

./upload-scene.sh --scene_id $SCENE_ID --scene_dir scene-test --env $ENV --verbose

./upload-object.sh --scene_id $SCENE_ID --object_filename cube --object_id cube --object_dir object-test --env $ENV --verbose

./upload-object-manifest.sh --scene_id $SCENE_ID --env $ENV --verbose

./upload-object.sh --scene_id $SCENE_ID --object_filename Lantern --object_id Lantern --object_dir lantern-test --env $ENV --verbose

./upload-object-manifest.sh --scene_id $SCENE_ID --env $ENV --verbose

./list-objects.sh --scene_id $SCENE_ID --env $ENV --verbose
