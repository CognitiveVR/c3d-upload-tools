package scene

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"time"

	"github.com/cognitivevr/c3d-upload-tools/internal/gateway"
)

// DefaultVersionWait is how long an upload waits for its new version to
// become visible.
const DefaultVersionWait = 30 * time.Second

var pollInterval = 500 * time.Millisecond

// Version is one scene version. ID is the integer id other routes take.
type Version struct {
	Number     int    `json:"versionNumber"`
	ID         int64  `json:"id"`
	SDKVersion string `json:"sdkVersion"`
}

// Info is one scene as GET /scenes/{sceneId} describes it.
type Info struct {
	ID          string    `json:"id"`
	SDKFacingID string    `json:"sdkFacingId"`
	SceneName   string    `json:"sceneName"`
	Versions    []Version `json:"versions"`
}

type wireScene struct {
	ID          string        `json:"id"`
	SDKFacingID string        `json:"sdkFacingId"`
	SceneName   string        `json:"sceneName"`
	Versions    []wireVersion `json:"versions"`
}

type wireVersion struct {
	ID            int64  `json:"id"`
	VersionNumber int    `json:"versionNumber"`
	SDKVersion    string `json:"sdkVersion"`
}

// Describe reads one scene. An id the key cannot reach comes back as a
// *gateway.Error of kind SceneNotAccessible once the client is preflighted.
func Describe(ctx context.Context, client *gateway.Client, sceneID string) (Info, error) {
	var wire wireScene
	if err := client.Get(ctx, "/scenes/"+url.PathEscape(sceneID), nil, &wire); err != nil {
		return Info{}, err
	}
	return wire.info(), nil
}

// List reads every scene the key can reach, in the order the gateway returns
// them. Each scene carries a non-nil Versions list.
func List(ctx context.Context, client *gateway.Client) ([]Info, error) {
	var wire []wireScene
	if err := client.Get(ctx, "/scenes", nil, &wire); err != nil {
		return nil, err
	}
	scenes := make([]Info, len(wire))
	for i, w := range wire {
		scenes[i] = w.info()
	}
	return scenes, nil
}

func (w wireScene) info() Info {
	info := Info{ID: w.ID, SDKFacingID: w.SDKFacingID, SceneName: w.SceneName, Versions: []Version{}}
	for _, v := range w.Versions {
		info.Versions = append(info.Versions, Version{Number: v.VersionNumber, ID: v.ID, SDKVersion: v.SDKVersion})
	}
	return info
}

// NewestFirst returns a copy of versions sorted by descending number.
func NewestFirst(versions []Version) []Version {
	sorted := slices.Clone(versions)
	slices.SortFunc(sorted, func(a, b Version) int { return b.Number - a.Number })
	return sorted
}

// LatestVersion is the version with the highest number, wherever it sits in
// the list.
func LatestVersion(info Info) (Version, bool) {
	var latest Version
	found := false
	for _, v := range info.Versions {
		if !found || v.Number > latest.Number {
			latest, found = v, true
		}
	}
	return latest, found
}

func VersionByNumber(info Info, n int) (Version, bool) {
	for _, v := range info.Versions {
		if v.Number == n {
			return v, true
		}
	}
	return Version{}, false
}

// ConcurrentVersionsError reports that more than one version newer than the
// one the upload started from appeared, so none can be attributed to it.
type ConcurrentVersionsError struct {
	SceneID    string
	Candidates []int
}

func (e *ConcurrentVersionsError) Error() string {
	return fmt.Sprintf("scene %s: versions %v appeared during the upload", e.SceneID, e.Candidates)
}

// VersionNotVisibleError reports that the expected version did not appear,
// either before the timeout (Cause nil) or because a lookup failed. A
// transient failure is the Cause only when the timeout ends the wait on it.
type VersionNotVisibleError struct {
	SceneID    string
	Expected   int
	LatestSeen int
	Cause      error
}

func (e *VersionNotVisibleError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("scene %s: reading version %d failed: %v", e.SceneID, e.Expected, e.Cause)
	}
	return fmt.Sprintf("scene %s: version %d is not visible yet (latest is %d)", e.SceneID, e.Expected, e.LatestSeen)
}

func (e *VersionNotVisibleError) Unwrap() error { return e.Cause }

// WaitForVersion polls the scene until a version numbered expected or higher
// appears, for at most timeout. Exactly one such version is returned; more
// than one is a *ConcurrentVersionsError; none is a *VersionNotVisibleError.
// latestSeen is the highest version number of the last successful poll. A
// lookup failing with 429, a 5xx or a network error counts as "not visible
// yet"; any other lookup failure ends the wait at once.
func WaitForVersion(ctx context.Context, client *gateway.Client, sceneID string, expected int, timeout time.Duration) (Version, int, error) {
	deadline := time.Now().Add(timeout)
	latestSeen := 0
	var lastErr error
	for {
		info, err := Describe(ctx, client, sceneID)
		lastErr = err
		if err != nil && !transientLookupError(err) {
			return Version{}, latestSeen, &VersionNotVisibleError{SceneID: sceneID, Expected: expected, LatestSeen: latestSeen, Cause: err}
		}
		if err == nil {
			if v, done, err := pickVersion(info, sceneID, expected, &latestSeen); done {
				return v, latestSeen, err
			}
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return Version{}, latestSeen, &VersionNotVisibleError{SceneID: sceneID, Expected: expected, LatestSeen: latestSeen, Cause: lastErr}
		}
		timer := time.NewTimer(min(pollInterval, remaining))
		select {
		case <-ctx.Done():
			timer.Stop()
			return Version{}, latestSeen, &VersionNotVisibleError{SceneID: sceneID, Expected: expected, LatestSeen: latestSeen, Cause: ctx.Err()}
		case <-timer.C:
		}
	}
}

// transientLookupError is a lookup failure that a later poll may not repeat:
// 429, any 5xx, or a network error.
func transientLookupError(err error) bool {
	var gerr *gateway.Error
	if !errors.As(err, &gerr) {
		return false
	}
	return gerr.Kind == gateway.Network || gerr.StatusCode == http.StatusTooManyRequests ||
		(gerr.StatusCode >= 500 && gerr.StatusCode <= 599)
}

// pickVersion records the latest number in info and reports whether the wait
// is over: one version at or above expected, or several.
func pickVersion(info Info, sceneID string, expected int, latestSeen *int) (Version, bool, error) {
	latest, _ := LatestVersion(info)
	*latestSeen = latest.Number
	var fresh []Version
	for _, v := range info.Versions {
		if v.Number >= expected {
			fresh = append(fresh, v)
		}
	}
	switch len(fresh) {
	case 0:
		return Version{}, false, nil
	case 1:
		return fresh[0], true, nil
	}
	sort.Slice(fresh, func(i, j int) bool { return fresh[i].Number < fresh[j].Number })
	candidates := make([]int, len(fresh))
	for i, v := range fresh {
		candidates[i] = v.Number
	}
	return Version{}, true, &ConcurrentVersionsError{SceneID: sceneID, Candidates: candidates}
}
