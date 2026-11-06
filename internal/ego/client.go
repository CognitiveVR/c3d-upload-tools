package ego

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/gateway"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/ui"
)

// UploadResult describes a completed upload. Model is the model file's base
// name and FileCount the number of parts sent.
type UploadResult struct {
	Slot      string
	Model     string
	FileCount int
}

// Listing is the set of filled slots of one scene version.
type Listing struct {
	SceneID string `json:"sceneId"`
	Version int    `json:"versionNumber"`
	Egos    []Slot `json:"egos"`
}

// Slot is one filled ego slot as the gateway reports it.
type Slot struct {
	Key       string   `json:"slot"`
	Model     string   `json:"model"`
	Files     []string `json:"files"`
	Manifest  bool     `json:"manifest"`
	Optimized bool     `json:"optimized"`
}

// DeleteResult says whether the slot held a model before the delete.
type DeleteResult struct {
	Slot    string
	Existed bool
}

// ListPath is the route that lists a scene's ego slots.
func ListPath(sceneID string) string {
	return "/scenes/" + url.PathEscape(sceneID) + "/egos"
}

// SlotPath is the route that uploads to and deletes one slot.
func SlotPath(sceneID, slot string) string {
	return ListPath(sceneID) + "/" + url.PathEscape(slot)
}

func versionQuery(versionNumber int) url.Values {
	return url.Values{"version": {strconv.Itoa(versionNumber)}}
}

// Upload sends plan to its slot in version versionNumber of scene sceneID,
// replacing whatever the slot held. log and progress may be nil.
func Upload(ctx context.Context, client *gateway.Client, plan *UploadPlan, sceneID string, versionNumber int, log *ui.Logger, progress *ui.Progress) (UploadResult, error) {
	if log == nil {
		log = ui.New(io.Discard, ui.Options{})
	}
	defer progress.Abort()
	if _, err := client.PostMultipart(ctx, SlotPath(sceneID, plan.Slot), versionQuery(versionNumber), plan.Parts, progress); err != nil {
		return UploadResult{}, err
	}
	progress.Done()
	result := UploadResult{Slot: plan.Slot, Model: filepath.Base(plan.Files.Paths[0]), FileCount: len(plan.Parts)}
	log.Info("Uploaded %s to slot %s of scene %s version %d", result.Model, plan.Slot, sceneID, versionNumber)
	return result, nil
}

type wireListing struct {
	SceneID string `json:"sceneId"`
	Version int    `json:"version"`
	Egos    []struct {
		Key       string          `json:"key"`
		Model     string          `json:"model"`
		Files     []string        `json:"files"`
		Manifest  json.RawMessage `json:"manifest"`
		Optimized bool            `json:"optimized"`
	} `json:"egos"`
}

// List reads the filled slots of version versionNumber of scene sceneID.
// Egos and every Files are non-nil.
func List(ctx context.Context, client *gateway.Client, sceneID string, versionNumber int) (Listing, error) {
	var wire wireListing
	if err := client.Get(ctx, ListPath(sceneID), versionQuery(versionNumber), &wire); err != nil {
		return Listing{}, err
	}
	listing := Listing{SceneID: wire.SceneID, Version: wire.Version, Egos: []Slot{}}
	if listing.SceneID == "" {
		listing.SceneID = sceneID
	}
	if listing.Version == 0 {
		listing.Version = versionNumber
	}
	for _, e := range wire.Egos {
		files := e.Files
		if files == nil {
			files = []string{}
		}
		listing.Egos = append(listing.Egos, Slot{
			Key: e.Key, Model: e.Model, Files: files, Manifest: manifestPresent(e.Manifest), Optimized: e.Optimized,
		})
	}
	return listing, nil
}

// manifestPresent reads the gateway's manifest field, which may be a string,
// an object or null.
func manifestPresent(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) && !bytes.Equal(trimmed, []byte("false"))
}

// Find returns the slot named key.
func (l Listing) Find(key string) (Slot, bool) {
	i := slices.IndexFunc(l.Egos, func(s Slot) bool { return s.Key == key })
	if i < 0 {
		return Slot{}, false
	}
	return l.Egos[i], true
}

// Delete empties slot in version versionNumber of scene sceneID. The gateway
// answers the same whether or not the slot held a model, so Existed comes
// from a listing read just before the delete.
func Delete(ctx context.Context, client *gateway.Client, sceneID, slot string, versionNumber int) (DeleteResult, error) {
	listing, err := List(ctx, client, sceneID, versionNumber)
	if err != nil {
		return DeleteResult{}, err
	}
	_, existed := listing.Find(slot)
	if err := client.Delete(ctx, SlotPath(sceneID, slot), versionQuery(versionNumber), nil); err != nil {
		return DeleteResult{}, err
	}
	return DeleteResult{Slot: slot, Existed: existed}, nil
}
