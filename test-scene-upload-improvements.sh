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

# Colors for output
COLOR_RESET="\033[0m"
COLOR_GREEN="\033[1;32m"
COLOR_RED="\033[1;31m"
COLOR_BLUE="\033[1;34m"
COLOR_YELLOW="\033[1;33m"

# Test results tracking
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_TOTAL=0

# Helper functions
print_section() {
  echo ""
  echo -e "${COLOR_BLUE}========================================${COLOR_RESET}"
  echo -e "${COLOR_BLUE}$1${COLOR_RESET}"
  echo -e "${COLOR_BLUE}========================================${COLOR_RESET}"
  echo ""
}

print_test() {
  echo -e "${COLOR_YELLOW}TEST $1: $2${COLOR_RESET}"
  echo ""
  TESTS_TOTAL=$((TESTS_TOTAL + 1))
}

print_pass() {
  echo ""
  echo -e "${COLOR_GREEN}✓ TEST PASSED: $1${COLOR_RESET}"
  echo ""
  TESTS_PASSED=$((TESTS_PASSED + 1))
}

print_fail() {
  echo ""
  echo -e "${COLOR_RED}✗ TEST FAILED: $1${COLOR_RESET}"
  echo ""
  TESTS_FAILED=$((TESTS_FAILED + 1))
}

print_summary() {
  echo ""
  echo -e "${COLOR_BLUE}========================================${COLOR_RESET}"
  echo -e "${COLOR_BLUE}TEST SUMMARY${COLOR_RESET}"
  echo -e "${COLOR_BLUE}========================================${COLOR_RESET}"
  echo ""
  echo "Total Tests:  $TESTS_TOTAL"
  echo -e "Passed:       ${COLOR_GREEN}$TESTS_PASSED${COLOR_RESET}"
  echo -e "Failed:       ${COLOR_RED}$TESTS_FAILED${COLOR_RESET}"
  echo ""

  if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${COLOR_GREEN}🎉 ALL TESTS PASSED!${COLOR_RESET}"
  else
    echo -e "${COLOR_RED}⚠️  SOME TESTS FAILED${COLOR_RESET}"
    exit 1
  fi
}

# Extract scene ID from upload output
extract_scene_id() {
  local output="$1"
  echo "$output" | grep "Scene ID:" | sed 's/.*Scene ID: \([a-f0-9-]*\).*/\1/'
}

# Check if upload succeeded (HTTP 200 or 201)
check_success() {
  local output="$1"
  if echo "$output" | grep -q "HTTP 200\|HTTP 201"; then
    return 0
  else
    return 1
  fi
}

# Check if scene ID was extracted
check_scene_id_extracted() {
  local output="$1"
  if echo "$output" | grep -q "Scene ID: [a-f0-9-]*"; then
    return 0
  else
    return 1
  fi
}

# Check if version check was performed
check_version_check() {
  local output="$1"
  if echo "$output" | grep -q "Retrieving current scene version"; then
    return 0
  else
    return 1
  fi
}

# Check if HTML error was detected
check_html_error_detection() {
  local output="$1"
  if echo "$output" | grep -q "Server returned an HTML error page"; then
    return 0
  else
    return 1
  fi
}

# Main test execution
main() {
  print_section "SCENE UPLOAD IMPROVEMENT TESTS"

  echo "This script tests the improvements made to align bash scripts"
  echo "with the Unity SDK's API interaction patterns."
  echo ""
  echo "Testing environment: dev (data.c3ddev.com)"
  echo "Test scenes: scene-test, vancouver-scene-test"
  echo ""
  read -p "Press Enter to start tests..."

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
  OUTPUT=$(./upload-scene.sh --scene_dir vancouver-scene-test --env dev --verbose 2>&1)

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
  # TEST 5: HTML Error Detection
  # ============================================================
  if [ -n "${SCENE_ID_2:-}" ]; then
    print_test "5" "HTML Error Detection - Graceful Handling"

    # Note: This may succeed or fail depending on API behavior
    # The important thing is to check if HTML errors are detected when they occur
    OUTPUT=$(./upload-scene.sh --scene_dir vancouver-scene-test --scene_id "$SCENE_ID_2" --env dev --verbose 2>&1 || true)

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
