package scene

import (
	"context"
	"fmt"
	"net/url"
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
	Number     int
	ID         int64
	SDKVersion string
}

// Info is one scene as GET /scenes/{sceneId} describes it.
type Info struct {
	ID          string
	SDKFacingID string
	SceneName   string
	Versions    []Version
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
	info := Info{ID: wire.ID, SDKFacingID: wire.SDKFacingID, SceneName: wire.SceneName}
	for _, v := range wire.Versions {
		info.Versions = append(info.Versions, Version{Number: v.VersionNumber, ID: v.ID, SDKVersion: v.SDKVersion})
	}
	return info, nil
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
// either before the timeout (Cause nil) or because a lookup failed.
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
// latestSeen is the highest version number of the last successful poll.
func WaitForVersion(ctx context.Context, client *gateway.Client, sceneID string, expected int, timeout time.Duration) (Version, int, error) {
	deadline := time.Now().Add(timeout)
	latestSeen := 0
	for {
		info, err := Describe(ctx, client, sceneID)
		if err != nil {
			return Version{}, latestSeen, &VersionNotVisibleError{SceneID: sceneID, Expected: expected, LatestSeen: latestSeen, Cause: err}
		}
		latest, _ := LatestVersion(info)
		latestSeen = latest.Number
		var fresh []Version
		for _, v := range info.Versions {
			if v.Number >= expected {
				fresh = append(fresh, v)
			}
		}
		switch len(fresh) {
		case 0:
		case 1:
			return fresh[0], latestSeen, nil
		default:
			sort.Slice(fresh, func(i, j int) bool { return fresh[i].Number < fresh[j].Number })
			candidates := make([]int, len(fresh))
			for i, v := range fresh {
				candidates[i] = v.Number
			}
			return Version{}, latestSeen, &ConcurrentVersionsError{SceneID: sceneID, Candidates: candidates}
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return Version{}, latestSeen, &VersionNotVisibleError{SceneID: sceneID, Expected: expected, LatestSeen: latestSeen}
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
