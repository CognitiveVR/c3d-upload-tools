package object

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"strconv"

	"github.com/cognitivevr/c3d-upload-tools/internal/gateway"
	"github.com/cognitivevr/c3d-upload-tools/internal/manifest"
	"github.com/cognitivevr/c3d-upload-tools/internal/ui"
)

// IDSource says where an object id came from.
type IDSource int

const (
	Flag IDSource = iota + 1
	ReusedFromManifest
	Generated
)

func (s IDSource) String() string {
	switch s {
	case Flag:
		return "from --object-id"
	case ReusedFromManifest:
		return "reused from the manifest"
	case Generated:
		return "generated"
	default:
		return fmt.Sprintf("IDSource(%d)", int(s))
	}
}

// UploadPath is the route that receives the files of the object named mesh.
func UploadPath(sceneID, mesh string) string {
	return ManifestPath(sceneID) + "/" + url.PathEscape(mesh)
}

// ManifestPath is the route that receives a scene's object manifest.
func ManifestPath(sceneID string) string {
	return "/objects/" + url.PathEscape(sceneID)
}

// VersionQuery selects a scene version.
func VersionQuery(versionNumber int) url.Values {
	return url.Values{"version": {strconv.Itoa(versionNumber)}}
}

// Upload sends plan to version versionNumber of scene sceneID. progress may be nil.
func Upload(ctx context.Context, client *gateway.Client, plan *UploadPlan, sceneID string, versionNumber int, log *ui.Logger, progress *ui.Progress) error {
	if log == nil {
		log = ui.New(io.Discard, ui.Options{})
	}
	defer progress.Abort()
	if _, err := client.PostMultipart(ctx, UploadPath(sceneID, plan.Mesh), VersionQuery(versionNumber), plan.Parts, progress); err != nil {
		return err
	}
	progress.Done()
	log.Info("Uploaded object %s to scene %s version %d", plan.Mesh, sceneID, versionNumber)
	return nil
}

// UploadManifest posts m to version versionNumber of scene sceneID. The
// gateway upserts by id and never deletes.
func UploadManifest(ctx context.Context, client *gateway.Client, m *manifest.Manifest, sceneID string, versionNumber int) error {
	body := *m
	if body.Objects == nil {
		body.Objects = []manifest.Entry{}
	}
	_, err := client.PostJSON(ctx, ManifestPath(sceneID), VersionQuery(versionNumber), body)
	return err
}

// ResolveID picks the id of the object whose mesh is mesh: flagID when set
// (it must be valid), else the id of the manifest entry with the same mesh,
// else a new UUID.
func ResolveID(flagID string, m *manifest.Manifest, mesh string) (string, IDSource, error) {
	if flagID != "" {
		if err := manifest.ValidateID(flagID); err != nil {
			return "", 0, fmt.Errorf("invalid --object-id: %w", err)
		}
		return flagID, Flag, nil
	}
	if m != nil {
		if id, ok := m.IDForMesh(mesh); ok {
			return id, ReusedFromManifest, nil
		}
	}
	return NewUUID(), Generated, nil
}

// NewUUID returns a random lowercase UUID v4.
func NewUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}
