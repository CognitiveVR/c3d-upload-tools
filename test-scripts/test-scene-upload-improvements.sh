#!/bin/bash

# test-scene-upload-improvements.sh
# Test script for scene upload improvements (Unity SDK alignment)
#
# This script validates:
# 1. Response format handling (plain text scene ID for new scenes, empty body for updates)
# 2. HTML error detection
# 3. Pre-upload version check
# 4. Strict response code validation (200/201 only)
# 5. No breaking changes to existing functionality

set -e
set -u

# Change to repo root directory (parent of test-scripts)
cd "$(dirname "$0")/.."

# Source common test utilities (from repo root)
source test-scripts/test-utils.sh

# Main test execution
main() {
  print_section "SCENE UPLOAD IMPROVEMENT TESTS"

  echo "This script tests the improvements made to align bash scripts"
  echo "with the Unity SDK's API interaction patterns."
  echo ""
  echo "Testing environment: dev (data.c3ddev.com)"
  echo "Test scenes: scene-test, test-scene-vancouver"
  echo ""
  read -p "Press Enter to start tests..."

  # Update scene names with current timestamp for unique test runs
  echo ""
  echo "Updating scene names with timestamps..."
  update_scene_timestamp "scene-test"
  update_scene_timestamp "test-scene-vancouver"
  echo ""

  # ============================================================
  # TEST 1: Dry Run (verify no breaking changes)
  # ============================================================
  print_test "1" "Dry Run - No Breaking Changes"

  OUTPUT=$(./upload-scene.sh --scene_dir scene-test --env dev --dry_run --verbose 2>&1)

  if echo "$OUTPUT" | grep -q "DRY RUN completed"; then
    print_pass "Dry run completed successfully, no breaking changes"
  else
    print_fail "Dry run failed"
    echo "$OUTPUT"
  fi

  # ============================================================
  # TEST 2: New Scene Upload (small scene)
  # ============================================================
  print_test "2" "New Scene Upload - Plain Text Scene ID Extraction"

  OUTPUT=$(./upload-scene.sh --scene_dir scene-test --env dev --verbose 2>&1)

  if check_success "$OUTPUT"; then
    if check_scene_id_extracted "$OUTPUT"; then
      SCENE_ID_1=$(extract_scene_id "$OUTPUT")
      echo "Extracted Scene ID: $SCENE_ID_1"

      if echo "$OUTPUT" | grep -q "💡 TIP: Save this Scene ID"; then
        print_pass "New scene created, scene ID extracted with helpful tips"
      else
        print_fail "Scene created but missing helpful tips"
      fi
    else
      print_fail "Scene created but scene ID not extracted"
      echo "$OUTPUT"
    fi
  else
    print_fail "Scene upload failed"
    echo "$OUTPUT"
  fi

  # ============================================================
  # TEST 3: Scene Update (with version check)
  # ============================================================
  if [ -n "${SCENE_ID_1:-}" ]; then
    print_test "3" "Scene Update - Pre-Upload Version Check"

    OUTPUT=$(./upload-scene.sh --scene_dir scene-test --scene_id "$SCENE_ID_1" --env dev --verbose 2>&1)

    if check_version_check "$OUTPUT"; then
      if check_success "$OUTPUT"; then
        print_pass "Scene updated with pre-upload version check executed"
      else
        print_fail "Version check executed but upload failed"
        echo "$OUTPUT"
      fi
    else
      print_fail "Pre-upload version check not executed"
      echo "$OUTPUT"
    fi
  else
    print_fail "Skipping Test 3 - no scene ID from Test 2"
  fi

  # ============================================================
  # TEST 4: Large Scene Upload (vancouver)
  # ============================================================
  print_test "4" "Large Scene Upload (76MB) - Performance Test"

  echo "Uploading large scene (this may take 30+ seconds)..."
  OUTPUT=$(./upload-scene.sh --scene_dir test-scene-vancouver --env dev --verbose 2>&1)

  if check_success "$OUTPUT"; then
    if check_scene_id_extracted "$OUTPUT"; then
      SCENE_ID_2=$(extract_scene_id "$OUTPUT")
      echo "Extracted Scene ID: $SCENE_ID_2"

      UPLOAD_TIME=$(echo "$OUTPUT" | grep "Upload completed in" | sed 's/.*Upload completed in \([0-9]*\) seconds.*/\1/')
      echo "Upload time: ${UPLOAD_TIME}s"

      print_pass "Large scene uploaded successfully"
    else
      print_fail "Large scene uploaded but scene ID not extracted"
      echo "$OUTPUT"
    fi
  else
    print_fail "Large scene upload failed"
    echo "$OUTPUT"
  fi

  # ============================================================
  # TEST 4.5: Textured Scene Upload (SciFiHelmet)
  # ============================================================
  print_test "4.5" "Textured Scene Upload (SciFiHelmet) - Performance Test"

  # Check if SciFiHelmet directory exists
  if [[ ! -d "SciFiHelmet" ]]; then
    print_fail "SciFiHelmet directory not found - skipping test"
  else
    echo "Uploading large textured scene (this may take 30+ seconds)..."

    # Disable exit-on-error temporarily to capture failures
    set +e
    OUTPUT=$(./upload-scene.sh --scene_dir SciFiHelmet --env dev --verbose 2>&1)
    UPLOAD_EXIT_CODE=$?
    set -e

    if [[ $UPLOAD_EXIT_CODE -eq 0 ]] && check_success "$OUTPUT"; then
      if check_scene_id_extracted "$OUTPUT"; then
        SCENE_ID_2_SCIFI=$(extract_scene_id "$OUTPUT")
        echo "Extracted Scene ID: $SCENE_ID_2_SCIFI"

        UPLOAD_TIME=$(echo "$OUTPUT" | grep "Upload completed in" | sed 's/.*Upload completed in \([0-9]*\) seconds.*/\1/')
        echo "Upload time: ${UPLOAD_TIME}s"

        print_pass "Textured scene uploaded successfully"
      else
        print_fail "Textured scene uploaded but scene ID not extracted"
        echo "$OUTPUT"
      fi
    else
      print_fail "Textured scene upload failed (exit code: $UPLOAD_EXIT_CODE)"
      echo "$OUTPUT"
    fi
  fi

  # ============================================================
  # TEST 5: HTML Error Detection
  # ============================================================
  if [ -n "${SCENE_ID_2:-}" ]; then
    print_test "5" "HTML Error Detection - Graceful Handling"

    # Note: This may succeed or fail depending on API behavior
    # The important thing is to check if HTML errors are detected when they occur
    OUTPUT=$(./upload-scene.sh --scene_dir test-scene-vancouver --scene_id "$SCENE_ID_2" --env dev --verbose 2>&1 || true)

    if check_html_error_detection "$OUTPUT"; then
      print_pass "HTML error page detected and handled gracefully"
    elif check_success "$OUTPUT"; then
      echo "Note: Upload succeeded (no HTML error to detect)"
      print_pass "Upload succeeded with proper version check"
    else
      # Check if it failed with proper error handling (not HTML)
      if echo "$OUTPUT" | grep -q "ERROR.*failed with status"; then
        print_pass "Error handled properly (non-HTML error response)"
      else
        print_fail "Error occurred but not handled properly"
        echo "$OUTPUT"
      fi
    fi
  else
    print_fail "Skipping Test 5 - no scene ID from Test 4"
  fi

  # ============================================================
  # TEST 6: Strict Response Code Validation
  # ============================================================
  print_test "6" "Response Code Validation - Only 200/201 Accepted"

  # This test verifies the code logic rather than API behavior
  # Check that the script only accepts 200 and 201
  if grep -q 'if \[\[ "$HTTP_STATUS" -eq 200 \]\] || \[\[ "$HTTP_STATUS" -eq 201 \]\]' upload-scene.sh; then
    print_pass "Response code validation correctly checks for 200 or 201 only"
  else
    print_fail "Response code validation may accept other 2xx codes"
  fi

  # ============================================================
  # TEST 7: Documentation and Unity SDK References
  # ============================================================
  print_test "7" "Documentation - Unity SDK References Present"

  if grep -q "Unity SDK Reference: EditorCore.cs" upload-scene.sh && \
     grep -q "Unity Reference: ExportUtility.cs" upload-scene.sh; then
    print_pass "Unity SDK references documented in code"
  else
    print_fail "Unity SDK references missing from documentation"
  fi

  # ============================================================
  # SUMMARY
  # ============================================================
  print_summary
}

# Run main function
main "$@"
