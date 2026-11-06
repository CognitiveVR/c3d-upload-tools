package ego

import (
	"context"
	"io"
	"time"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/gateway"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/ui"
)

const minimumPollInterval = time.Millisecond

// WaitOptimized lists the version's slots every interval (at least a
// millisecond) until slot is marked optimized or timeout elapses; timeout
// also bounds every poll and its retries. Running out of time returns false
// and no error, because the gateway never marks small models optimized. log
// may be nil.
func WaitOptimized(ctx context.Context, client *gateway.Client, sceneID, slot string, versionNumber int, interval, timeout time.Duration, log *ui.Logger) (bool, error) {
	if log == nil {
		log = ui.New(io.Discard, ui.Options{})
	}
	interval = max(interval, minimumPollInterval)
	deadline := time.Now().Add(timeout)
	pollCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	for {
		timer := time.NewTimer(max(min(interval, time.Until(deadline)), 0))
		select {
		case <-pollCtx.Done():
			timer.Stop()
			return false, ctx.Err()
		case <-timer.C:
		}
		if pollCtx.Err() != nil {
			return false, ctx.Err()
		}
		listing, err := List(pollCtx, client, sceneID, versionNumber)
		if err != nil {
			if pollCtx.Err() != nil {
				return false, ctx.Err()
			}
			return false, err
		}
		if s, ok := listing.Find(slot); ok && s.Optimized {
			return true, nil
		}
		log.Debug("slot %s of scene %s version %d is not optimized yet", slot, sceneID, versionNumber)
	}
}
