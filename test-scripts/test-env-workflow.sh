#!/bin/bash

# Test Environment Workflow Script
#
# Tests the complete Cognitive3D upload workflow with environment-specific configuration.
# Copies appropriate .env.sample.* file to .env and runs full upload test suite.

set -euo pipefail

# Change to repo root directory (parent of test-scripts)
cd "$(dirname "$0")/.."

# Source utilities for logging and environment loading
source ./upload-utils.sh

# Initialize script
script_start_time=$(date +%s)
log_info "Starting Cognitive3D environment workflow test"

show_help() {
    cat << EOF
Test Environment Workflow Script

Tests the complete Cognitive3D upload workflow with environment-specific configuration.

USAGE:
    $0 --env <prod|dev> [--verbose] [--dry_run] [--help]

PARAMETERS:
    --env <prod|dev>    Target environment for testing
                        - 'prod': Uses .env.sample.prod configuration
                        - 'dev': Uses .env.sample.dev configuration

OPTIONS:
    --verbose           Enable detailed logging and debug output
    --dry_run          Preview operations without executing uploads
    --help             Show this help message

WORKFLOW:
    1. Copies .env.sample.<env> to .env
    2. Uploads test scene to get scene ID
    3. Adds scene ID to .env file
    4. Tests object upload with environment variable fallback
    5. Tests object manifest upload
    6. Lists objects to verify
    7. Cleans up temporary .env file

EXAMPLES:
    # Test dev environment workflow
    $0 --env dev --verbose

    # Test prod environment with dry run
    $0 --env prod --dry_run

    # Quick prod test
    $0 --env prod

REQUIREMENTS:
    - .env.sample.dev and .env.sample.prod files must exist
    - scene-test/ directory with test scene files
    - object-test/ directory with test object files
    - Dependencies: jq, curl

EOF
}

# Parse command line arguments
ENV=""
VERBOSE=false
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --env)
            ENV="$2"
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
            show_help
            exit 0
            ;;
        *)
            log_error "Unknown parameter: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Validate required parameters
if [[ -z "$ENV" ]]; then
    log_error "Missing required parameter: --env"
    echo "Use --help for usage information"
    exit 1
fi

if [[ "$ENV" != "prod" && "$ENV" != "dev" ]]; then
    log_error "Invalid environment: $ENV. Must be 'prod' or 'dev'"
    exit 1
fi

# Set verbose mode if requested
if [[ "$VERBOSE" = true ]]; then
    export C3D_VERBOSE=true
fi

# Validate required files
ENV_SAMPLE_FILE=".env.sample.$ENV"
if [[ ! -f "$ENV_SAMPLE_FILE" ]]; then
    log_error "Environment sample file not found: $ENV_SAMPLE_FILE"
    exit 1
fi

if [[ ! -d "scene-test" ]]; then
    log_error "Test scene directory not found: scene-test/"
    exit 1
fi

if [[ ! -d "object-test" ]]; then
    log_error "Test object directory not found: object-test/"
    exit 1
fi

log_info "Testing environment workflow for: $ENV"

# Backup existing .env if it exists
ENV_BACKUP=""
if [[ -f ".env" ]]; then
    ENV_BACKUP=".env.backup.$(date +%s)"
    log_info "Backing up existing .env to $ENV_BACKUP"
    if [[ "$DRY_RUN" = false ]]; then
        cp .env "$ENV_BACKUP"
    fi
fi

# Step 1: Copy environment sample to .env
log_info "Step 1: Setting up .env file from $ENV_SAMPLE_FILE"
if [[ "$DRY_RUN" = false ]]; then
    cp "$ENV_SAMPLE_FILE" .env
    log_info "Copied $ENV_SAMPLE_FILE to .env"
else
    log_info "[DRY_RUN] Would copy $ENV_SAMPLE_FILE to .env"
fi

if [[ "$DRY_RUN" = true ]]; then
    log_info "[DRY_RUN] Would run the following workflow:"
    log_info "[DRY_RUN] 1. ./upload-scene.sh --scene_dir scene-test --env $ENV"
    log_info "[DRY_RUN] 2. Extract scene ID and add to .env file"
    log_info "[DRY_RUN] 3. ./upload-object.sh --object_filename cube --object_dir object-test --env $ENV"
    log_info "[DRY_RUN] 4. ./upload-object-manifest.sh --env $ENV"
    log_info "[DRY_RUN] 5. ./list-objects.sh --env $ENV"
    
    # Restore backup if needed
    if [[ -n "$ENV_BACKUP" ]]; then
        log_info "[DRY_RUN] Would restore .env from backup: $ENV_BACKUP"
    fi
    exit 0
fi

# Function to cleanup on exit
cleanup() {
    local exit_code=$?
    log_info "Cleaning up..."
    
    # Restore original .env if we had a backup
    if [[ -n "$ENV_BACKUP" && -f "$ENV_BACKUP" ]]; then
        mv "$ENV_BACKUP" .env
        log_info "Restored original .env file"
    elif [[ -f ".env" ]]; then
        # Remove the temporary .env file if no backup existed
        rm .env
        log_info "Removed temporary .env file"
    fi
    
    local end_time=$(date +%s)
    local duration=$((end_time - script_start_time))
    log_info "Test completed in ${duration}s with exit code $exit_code"
    
    exit $exit_code
}

# Set up cleanup on script exit
trap cleanup EXIT

# Step 2: Upload scene to get scene ID
log_info "Step 2: Uploading test scene to get scene ID"
scene_output=$(./upload-scene.sh --scene_dir scene-test --env "$ENV" 2>&1)
scene_exit_code=$?

if [[ $scene_exit_code -ne 0 ]]; then
    log_error "Scene upload failed with exit code $scene_exit_code"
    echo "$scene_output"
    exit $scene_exit_code
fi

# Extract scene ID from output
scene_id=$(echo "$scene_output" | grep -E "Scene ID:|Scene created successfully with ID:" | tail -1 | sed -E 's/.*(([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})).*/\1/')

if [[ -z "$scene_id" ]]; then
    log_error "Could not extract scene ID from upload-scene.sh output"
    echo "Scene upload output:"
    echo "$scene_output"
    exit 1
fi

log_info "Scene uploaded successfully. Scene ID: $scene_id"

# Step 3: Add scene ID to .env file
log_info "Step 3: Adding scene ID to .env file"
echo "" >> .env
echo "# Scene ID from test upload ($(date))" >> .env
echo "C3D_SCENE_ID=$scene_id" >> .env
log_info "Added C3D_SCENE_ID=$scene_id to .env file"

# Step 4: Test object upload using environment variable
log_info "Step 4: Testing object upload with environment variable fallback"
./upload-object.sh --object_filename cube --object_dir object-test --env "$ENV"
log_info "Object upload completed successfully"

# Step 5: Test object manifest upload
log_info "Step 5: Testing object manifest upload"
./upload-object-manifest.sh --env "$ENV"
log_info "Object manifest upload completed successfully"

# Step 6: List objects to verify
log_info "Step 6: Listing objects to verify uploads"
./list-objects.sh --env "$ENV"
log_info "Object listing completed successfully"

log_info "✅ Environment workflow test completed successfully for $ENV environment"
log_info "✅ Verified .env file loading and C3D_SCENE_ID environment variable fallback"