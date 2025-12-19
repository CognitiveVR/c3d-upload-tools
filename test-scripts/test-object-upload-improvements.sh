#!/bin/bash

# test-object-upload-improvements.sh
# Test script for dynamic object upload improvements (Unity SDK alignment)
#
# This script validates:
# 1. Pre-upload version check for objects
# 2. Version parameter in object upload URLs
# 3. Manifest accumulation (merge not overwrite)
# 4. Manifest upload with version parameter
# 5. Duplicate object ID handling
# 6. Dry-run mode URL validation
# 7. Re-upload same objects (version consistency)
# 8. Re-upload manifest (idempotency)

set -e
set -u

# Change to repo root directory (parent of test-scripts)
cd "$(dirname "$0")/.."

# Source common test utilities (from repo root)
source test-scripts/test-utils.sh

# ============================================================
# Test-specific helper functions
# ============================================================

# Check manifest object count (specific to object upload tests)
get_manifest_object_count() {
  local manifest_file="$1"
  if [ -f "$manifest_file" ]; then
    jq '.objects | length' "$manifest_file"
  else
    echo "0"
  fi
}

# Main test execution
main() {
  print_section "DYNAMIC OBJECT UPLOAD IMPROVEMENT TESTS"

  echo "This script tests the improvements made to align bash scripts"
  echo "with the Unity SDK's API interaction patterns for dynamic objects."
  echo ""
  echo "Testing environment: dev (data.c3ddev.com)"
  echo "Test objects: cube, Lantern"
  echo ""
  read -p "Press Enter to start tests..."

  # ============================================================
  # TEST 0: Create test scene
  # ============================================================
  print_test "0" "Setup - Create Test Scene"

  OUTPUT=$(./upload-scene.sh --scene_dir scene-test --env dev --verbose 2>&1)

  if check_success "$OUTPUT"; then
    SCENE_ID=$(extract_scene_id "$OUTPUT")
    if [ -n "$SCENE_ID" ]; then
      echo "Test scene created: $SCENE_ID"
      print_pass "Test scene created successfully"
    else
      print_fail "Scene created but couldn't extract scene ID"
      echo "$OUTPUT"
      exit 1
    fi
  else
    print_fail "Failed to create test scene"
    echo "$OUTPUT"
    exit 1
  fi

  MANIFEST_FILE="${SCENE_ID}_object_manifest.json"

  # ============================================================
  # TEST 1: Single Object Upload with Version Check
  # ============================================================
  print_test "1" "Single Object Upload - Version Check & Version Parameter"

  OUTPUT=$(./upload-object.sh --scene_id "$SCENE_ID" --object_filename cube --object_dir object-test --env dev --verbose 2>&1)

  if check_version_check "$OUTPUT"; then
    if check_version_parameter "$OUTPUT"; then
      if check_success "$OUTPUT"; then
        OBJECT_COUNT=$(get_manifest_object_count "$MANIFEST_FILE")
        if [ "$OBJECT_COUNT" -eq 1 ]; then
          print_pass "Object uploaded with version check and version parameter, manifest created with 1 object"
        else
          print_fail "Object uploaded but manifest has $OBJECT_COUNT objects (expected 1)"
          cat "$MANIFEST_FILE"
        fi
      else
        print_fail "Version check executed but upload failed"
        echo "$OUTPUT"
      fi
    else
      print_fail "Version check executed but version parameter missing from URL"
      echo "$OUTPUT"
    fi
  else
    print_fail "Pre-upload version check not executed"
    echo "$OUTPUT"
  fi

  # ============================================================
  # TEST 2: Multiple Objects - Manifest Accumulation
  # ============================================================
  print_test "2" "Multiple Objects - Manifest Accumulation (Not Overwrite)"

  OUTPUT=$(./upload-object.sh --scene_id "$SCENE_ID" --object_filename Lantern --object_dir lantern-test --env dev --verbose 2>&1)

  if check_success "$OUTPUT"; then
    OBJECT_COUNT=$(get_manifest_object_count "$MANIFEST_FILE")
    if [ "$OBJECT_COUNT" -eq 2 ]; then
      # Verify both object IDs are present
      OBJECT_IDS=$(jq -r '.objects[].id' "$MANIFEST_FILE" | tr '\n' ',' | sed 's/,$//')
      if [[ "$OBJECT_IDS" == *"cube"* ]] && [[ "$OBJECT_IDS" == *"Lantern"* ]]; then
        print_pass "Second object merged into manifest (2 objects: cube, Lantern)"
      else
        print_fail "Manifest has 2 objects but IDs are wrong: $OBJECT_IDS"
        cat "$MANIFEST_FILE"
      fi
    else
      print_fail "Expected 2 objects in manifest, found $OBJECT_COUNT"
      cat "$MANIFEST_FILE"
    fi
  else
    print_fail "Second object upload failed"
    echo "$OUTPUT"
  fi

  # ============================================================
  # TEST 3: Manifest Upload with Version
  # ============================================================
  print_test "3" "Manifest Upload - Pre-Upload Version Check & Version Parameter"

  OUTPUT=$(./upload-object-manifest.sh --scene_id "$SCENE_ID" --env dev --verbose 2>&1)

  if check_version_check "$OUTPUT"; then
    if check_version_parameter "$OUTPUT"; then
      if check_success "$OUTPUT"; then
        print_pass "Manifest uploaded with version check and version parameter"
      else
        print_fail "Version check executed but manifest upload failed"
        echo "$OUTPUT"
      fi
    else
      print_fail "Version check executed but version parameter missing from URL"
      echo "$OUTPUT"
    fi
  else
    print_fail "Pre-upload version check not executed for manifest"
    echo "$OUTPUT"
  fi

  # ============================================================
  # TEST 4: Duplicate Object ID Handling
  # ============================================================
  print_test "4" "Duplicate Object ID - Update Not Duplicate"

  OUTPUT=$(./upload-object.sh --scene_id "$SCENE_ID" --object_filename cube --object_dir object-test --env dev --verbose 2>&1)

  if echo "$OUTPUT" | grep -q "already exists in manifest"; then
    OBJECT_COUNT=$(get_manifest_object_count "$MANIFEST_FILE")
    if [ "$OBJECT_COUNT" -eq 2 ]; then
      print_pass "Duplicate object ID handled correctly (updated entry, still 2 objects)"
    else
      print_fail "Duplicate detected but manifest has $OBJECT_COUNT objects (expected 2)"
      cat "$MANIFEST_FILE"
    fi
  else
    print_fail "Duplicate object ID not detected"
    echo "$OUTPUT"
  fi

  # ============================================================
  # TEST 5: Dry-Run Mode - Version Parameter Validation
  # ============================================================
  print_test "5" "Dry-Run Mode - Version Parameters in URLs"

  OUTPUT_OBJ=$(./upload-object.sh --scene_id "$SCENE_ID" --object_filename cube --object_dir object-test --env dev --dry_run --verbose 2>&1)
  OUTPUT_MAN=$(./upload-object-manifest.sh --scene_id "$SCENE_ID" --env dev --dry_run --verbose 2>&1)

  OBJ_HAS_VERSION=false
  MAN_HAS_VERSION=false

  if echo "$OUTPUT_OBJ" | grep -q "cube?version="; then
    OBJ_HAS_VERSION=true
  fi

  if echo "$OUTPUT_MAN" | grep -q "?version="; then
    MAN_HAS_VERSION=true
  fi

  if [ "$OBJ_HAS_VERSION" = true ] && [ "$MAN_HAS_VERSION" = true ]; then
    print_pass "Both object and manifest URLs include version parameter in dry-run"
  else
    if [ "$OBJ_HAS_VERSION" = false ]; then
      echo "Object URL missing version parameter:"
      echo "$OUTPUT_OBJ" | grep -E "Upload URL|curl.*objects"
    fi
    if [ "$MAN_HAS_VERSION" = false ]; then
      echo "Manifest URL missing version parameter:"
      echo "$OUTPUT_MAN" | grep -E "Endpoint:|curl.*objects"
    fi
    print_fail "Version parameters missing in dry-run URLs"
  fi

  # ============================================================
  # TEST 6: Manifest Format - 4 Decimal Places
  # ============================================================
  print_test "6" "Manifest Format - Float Precision (Unity SDK Format)"

  # Check if manifest has 4 decimal place formatting
  if grep -q '"1.0000"' "$MANIFEST_FILE" || grep -q '1.0000' "$MANIFEST_FILE"; then
    print_pass "Manifest uses 4 decimal place formatting (Unity SDK format)"
  else
    print_fail "Manifest does not use 4 decimal place formatting"
    echo "Expected format: 1.0000, 0.0000"
    echo "Actual manifest:"
    cat "$MANIFEST_FILE"
  fi

  # ============================================================
  # TEST 7: Re-upload Same Objects - Update Test
  # ============================================================
  print_test "7" "Re-upload Same Objects - Version Consistency"

  echo "Re-uploading cube (should update existing entry)..."
  OUTPUT_REUPLOAD=$(./upload-object.sh --scene_id "$SCENE_ID" --object_filename cube --object_dir object-test --env dev --verbose 2>&1)

  if check_success "$OUTPUT_REUPLOAD"; then
    # Verify it still uploaded to version 1
    if echo "$OUTPUT_REUPLOAD" | grep -q "scene version: 1"; then
      OBJECT_COUNT=$(get_manifest_object_count "$MANIFEST_FILE")
      if [ "$OBJECT_COUNT" -eq 2 ]; then
        print_pass "Object re-uploaded successfully, manifest still has 2 objects (updated, not duplicated)"
      else
        print_fail "Expected 2 objects in manifest after re-upload, found $OBJECT_COUNT"
      fi
    else
      print_fail "Re-upload may have gone to wrong version"
      echo "$OUTPUT_REUPLOAD" | grep -E "version|Version"
    fi
  else
    print_fail "Object re-upload failed"
    echo "$OUTPUT_REUPLOAD"
  fi

  # ============================================================
  # TEST 8: Re-upload Manifest - Idempotency Test
  # ============================================================
  print_test "8" "Re-upload Manifest - Idempotency"

  OUTPUT_MAN_REUPLOAD=$(./upload-object-manifest.sh --scene_id "$SCENE_ID" --env dev --verbose 2>&1)

  if check_version_check "$OUTPUT_MAN_REUPLOAD"; then
    if echo "$OUTPUT_MAN_REUPLOAD" | grep -q "scene version: 1"; then
      if check_success "$OUTPUT_MAN_REUPLOAD"; then
        print_pass "Manifest re-uploaded successfully to same version"
      else
        print_fail "Manifest re-upload failed"
        echo "$OUTPUT_MAN_REUPLOAD"
      fi
    else
      print_fail "Manifest may have uploaded to wrong version"
      echo "$OUTPUT_MAN_REUPLOAD" | grep -E "version|Version"
    fi
  else
    print_fail "Version check not executed for manifest re-upload"
    echo "$OUTPUT_MAN_REUPLOAD"
  fi

  # ============================================================
  # CLEANUP
  # ============================================================
  echo ""
  echo "Cleaning up test manifest file..."
  rm -f "$MANIFEST_FILE"

  # ============================================================
  # SUMMARY
  # ============================================================
  print_summary

  # ============================================================
  # DASHBOARD LINK
  # ============================================================
  echo ""
  echo -e "${COLOR_BLUE}========================================${COLOR_RESET}"
  echo -e "${COLOR_BLUE}View Test Results in Dashboard${COLOR_RESET}"
  echo -e "${COLOR_BLUE}========================================${COLOR_RESET}"
  echo ""
  echo "Scene ID: $SCENE_ID"
  echo ""
  echo "View uploaded objects:"
  echo "  https://app.c3ddev.com/v3/scenes/$SCENE_ID/v/1/objectlist"
  echo ""
}

# Run main function
main "$@"
