package ego

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/cognitivevr/c3d-upload-tools/internal/gateway"
)

func TestWaitOptimizedReturnsTrueOnceTheSlotFlips(t *testing.T) {
	fake, client := egoFake(t)
	fake.SetOptimizedAfterPolls(egoSceneID, 1, "head", 2)
	uploadHead(t, client, 1)
	optimized, err := WaitOptimized(context.Background(), client, egoSceneID, "head", 1, time.Millisecond, time.Minute, nil)
	if err != nil || !optimized {
		t.Fatalf("WaitOptimized = %v, %v; want true, nil", optimized, err)
	}
	var polls int
	for _, r := range fake.Requests() {
		if r.Method == http.MethodGet && r.Path == "/v0/scenes/"+egoSceneID+"/egos" {
			polls++
			if r.Query.Get("version") != "1" {
				t.Errorf("poll query = %v", r.Query)
			}
		}
	}
	if polls != 2 {
		t.Errorf("%d polls, want 2", polls)
	}
}

func TestWaitOptimizedTimingOutIsNotAnError(t *testing.T) {
	fake, client := egoFake(t)
	uploadHead(t, client, 1)
	optimized, err := WaitOptimized(context.Background(), client, egoSceneID, "head", 1, time.Millisecond, 20*time.Millisecond, nil)
	if err != nil || optimized {
		t.Fatalf("WaitOptimized = %v, %v; want false, nil", optimized, err)
	}
	if n := len(fake.Requests()); n < 2 {
		t.Errorf("%d requests, want the upload and at least one poll", n)
	}
}

func TestWaitOptimizedReturnsTheContextErrorWhenCancelled(t *testing.T) {
	_, client := egoFake(t)
	uploadHead(t, client, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	optimized, err := WaitOptimized(ctx, client, egoSceneID, "head", 1, time.Millisecond, time.Minute, nil)
	if optimized || !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitOptimized = %v, %v; want false, context.Canceled", optimized, err)
	}
}

func TestWaitOptimizedReportsAFailedPoll(t *testing.T) {
	fake, client := egoFake(t)
	uploadHead(t, client, 1)
	fake.FailWith(http.StatusForbidden, "no")
	optimized, err := WaitOptimized(context.Background(), client, egoSceneID, "head", 1, time.Millisecond, time.Minute, nil)
	var gerr *gateway.Error
	if optimized || !errors.As(err, &gerr) || gerr.Kind != gateway.Forbidden {
		t.Fatalf("WaitOptimized = %v, %v; want false and the gateway error", optimized, err)
	}
}
