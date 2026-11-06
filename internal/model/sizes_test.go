package model

import (
	"strings"
	"testing"
)

func TestCheckSizesWarnsPerFileAtFiveHundredMegabytes(t *testing.T) {
	warnings, err := CheckSizes([]SizedFile{{"big.glb", 500 * 1000 * 1000}, {"small.png", 499 * 1000 * 1000}}, 999*1000*1000)
	if err != nil || len(warnings) != 1 || !strings.Contains(warnings[0], "big.glb") || !strings.Contains(warnings[0], "500 MB") {
		t.Errorf("warnings %q, err %v", warnings, err)
	}
}

func TestCheckSizesWarnsAtTwoGiBAndRefusesAboveThree(t *testing.T) {
	if w, err := CheckSizes(nil, 2<<30); err != nil || len(w) != 1 {
		t.Errorf("2 GiB: %q, %v", w, err)
	}
	if w, err := CheckSizes(nil, 3<<30); err != nil || len(w) != 1 {
		t.Errorf("3 GiB: %q, %v", w, err)
	}
	if _, err := CheckSizes(nil, 3<<30+1); err == nil || !strings.Contains(err.Error(), "3 GiB") {
		t.Errorf("over 3 GiB: %v", err)
	}
}
