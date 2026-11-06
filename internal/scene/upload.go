package scene

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/cognitivevr/c3d-upload-tools/internal/gateway"
	"github.com/cognitivevr/c3d-upload-tools/internal/ui"
)

var versionWait = DefaultVersionWait

var createdSceneID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)

// Result is the outcome of a scene upload. When ConcurrentVersions is true
// the upload cannot be told from another one, so VersionNumber and VersionID
// are zero and CandidateVersions lists the versions that appeared.
type Result struct {
	SceneID            string `json:"sceneId"`
	Created            bool   `json:"created"`
	VersionNumber      int    `json:"versionNumber,omitempty"`
	VersionID          int64  `json:"versionId,omitempty"`
	CandidateVersions  []int  `json:"candidateVersions,omitempty"`
	ConcurrentVersions bool   `json:"concurrentVersions,omitempty"`
}

// UploadPath is the route that creates a scene (sceneID empty) or a new
// version of sceneID.
func UploadPath(sceneID string) string {
	if sceneID == "" {
		return "/scenes"
	}
	return "/scenes/" + url.PathEscape(sceneID)
}

// Upload sends plan as a new scene (sceneID empty) or as a new version of
// sceneID, whose latest version before the upload was previousLatest. Once
// the gateway accepts it, settings.json is written into the scene directory
// and Upload waits for the new version to appear. Any error after the gateway
// accepted the upload comes back with the Result filled in as far as it is
// known, so callers can still report the scene id.
func Upload(ctx context.Context, client *gateway.Client, plan *UploadPlan, sceneID string, previousLatest int, log *ui.Logger, progress *ui.Progress) (Result, error) {
	if log == nil {
		log = ui.New(io.Discard, ui.Options{})
	}
	defer progress.Abort()
	resp, err := client.PostMultipart(ctx, UploadPath(sceneID), nil, plan.Parts, progress)
	if err != nil {
		return Result{}, err
	}
	progress.Done()

	result := Result{SceneID: sceneID, Created: sceneID == ""}
	if result.Created {
		id := strings.TrimSpace(string(resp.Body))
		if !createdSceneID.MatchString(id) {
			return Result{Created: true}, fmt.Errorf("the gateway accepted the new scene but answered with an unusable scene id %q", id)
		}
		result.SceneID = id
	}
	if err := WriteSettings(plan.Local.Dir, plan.SettingsBytes); err != nil {
		return result, fmt.Errorf("the gateway accepted the upload as scene %s, but settings.json could not be written to %s: %w",
			result.SceneID, plan.Local.Dir, err)
	}
	log.Info("Uploaded to scene %s; waiting for the new version to appear", result.SceneID)

	version, _, err := WaitForVersion(ctx, client, result.SceneID, previousLatest+1, versionWait)
	var concurrent *ConcurrentVersionsError
	switch {
	case errors.As(err, &concurrent):
		result.ConcurrentVersions = true
		result.CandidateVersions = concurrent.Candidates
		err = nil
	case err == nil:
		result.VersionNumber = version.Number
		result.VersionID = version.ID
		log.Info("Scene %s version %d is ready", result.SceneID, version.Number)
	}
	if previousLatest > 0 {
		if result.VersionNumber > 0 {
			log.Warn("objects, manifests and egos belong to a scene version; upload them again for version %d", result.VersionNumber)
		} else {
			log.Warn("objects, manifests and egos belong to a scene version; upload them again for the new version")
		}
	}
	return result, err
}
