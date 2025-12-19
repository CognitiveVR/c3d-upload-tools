#!/bin/bash

# test-utils.sh
# Common utility functions for test scripts
#
# This file contains shared functions used across multiple test scripts:
# - Test output formatting (colors, sections, results)
# - Test result tracking
# - Scene ID extraction with ANSI code handling
# - Common validation checks

# ============================================================
# Color Constants
# ============================================================
COLOR_RESET="\033[0m"
COLOR_GREEN="\033[1;32m"
COLOR_RED="\033[1;31m"
COLOR_BLUE="\033[1;34m"
COLOR_YELLOW="\033[1;33m"

# ============================================================
# Test Results Tracking
# ============================================================
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_TOTAL=0

# ============================================================
# Test Output Functions
# ============================================================

# Print a section header
print_section() {
  echo ""
  echo -e "${COLOR_BLUE}========================================${COLOR_RESET}"
  echo -e "${COLOR_BLUE}$1${COLOR_RESET}"
  echo -e "${COLOR_BLUE}========================================${COLOR_RESET}"
  echo ""
}

# Print test start message
print_test() {
  echo -e "${COLOR_YELLOW}TEST $1: $2${COLOR_RESET}"
  echo ""
  TESTS_TOTAL=$((TESTS_TOTAL + 1))
}

# Print test pass message
print_pass() {
  echo ""
  echo -e "${COLOR_GREEN}✓ TEST PASSED: $1${COLOR_RESET}"
  echo ""
  TESTS_PASSED=$((TESTS_PASSED + 1))
}

# Print test fail message
print_fail() {
  echo ""
  echo -e "${COLOR_RED}✗ TEST FAILED: $1${COLOR_RESET}"
  echo ""
  TESTS_FAILED=$((TESTS_FAILED + 1))
}

# Print test summary and exit with appropriate code
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

# ============================================================
# Scene ID Extraction
# ============================================================

# Extract scene ID from upload output
# This function handles ANSI color codes that grep may inject
extract_scene_id() {
  local output="$1"
  # Comprehensive ANSI stripping and UUID extraction
  # 1. Strip ALL ANSI escape sequences (comprehensive pattern)
  # 2. Extract UUID pattern with grep --color=never to prevent grep from adding colors
  # 3. Clean any remaining whitespace or invisible characters
  local scene_id=$(echo "$output" | \
    sed 's/\x1b\[[0-9;]*[mKHfJABCDsu]//g' | \
    grep --color=never -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | \
    head -1 | \
    tr -d '[:space:]')

  if [[ -z "$scene_id" ]]; then
    echo "[ERROR] Failed to extract scene ID from output" >&2
    echo "[DEBUG] Searching for UUID pattern after cleaning" >&2
    echo "[DEBUG] First 50 lines of output:" >&2
    echo "$output" | head -50 >&2
    return 1
  fi

  echo "$scene_id"
}

# ============================================================
# Common Validation Checks
# ============================================================

# Check if upload succeeded (HTTP 200 or 201)
check_success() {
  local output="$1"
  if echo "$output" | grep -q "HTTP 200\|HTTP 201"; then
    return 0
  else
    return 1
  fi
}

# Check if scene ID was extracted from output
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

# Check if version parameter is in URL
check_version_parameter() {
  local output="$1"
  if echo "$output" | grep -q "version="; then
    return 0
  else
    return 1
  fi
}

# ============================================================
# Scene Timestamp Update
# ============================================================

# Update scene name with current timestamp
update_scene_timestamp() {
  local scene_dir="$1"
  local settings_file="$scene_dir/settings.json"

  if [[ ! -f "$settings_file" ]]; then
    echo "Warning: settings.json not found at $settings_file"
    return 1
  fi

  # Generate ISO8601 timestamp (e.g., 2025-12-19T17:30:45Z)
  local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

  # Read current scene name
  local current_name=$(jq -r '.sceneName' "$settings_file")

  # Strip any existing date/timestamp suffix (pattern: YYYY-MM-DD* or YYYY-MM-DDTHH:MM:SSZ)
  # Keep the base name before any date pattern
  local base_name=$(echo "$current_name" | sed -E 's/ [0-9]{4}-[0-9]{2}-[0-9]{2}[^ ]*$//')

  # Append new timestamp
  local new_name="${base_name} ${timestamp}"

  # Update settings.json with new scene name
  jq --arg name "$new_name" '.sceneName = $name' "$settings_file" > "${settings_file}.tmp" && \
    mv "${settings_file}.tmp" "$settings_file"

  echo "Updated scene name: $new_name"
}
