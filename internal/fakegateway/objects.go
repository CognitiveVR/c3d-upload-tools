package fakegateway

import (
	"context"
	"encoding/json"
	"net/http"
	"path"
	"strconv"
	"strings"
)

type versionKey struct {
	sceneID string
	number  int
}

type meshKey struct {
	versionKey
	mesh string
}

type objectFile struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type manifestEntry struct {
	ID              *string   `json:"id"`
	Mesh            *string   `json:"mesh"`
	Name            *string   `json:"name"`
	ScaleCustom     []float64 `json:"scaleCustom"`
	InitialPosition []float64 `json:"initialPosition"`
	InitialRotation []float64 `json:"initialRotation"`
	InitialScale    []float64 `json:"initialScale"`
	IsController    *bool     `json:"isController"`
	Scale           *float64  `json:"scale"`
	ExternalID      *string   `json:"externalId"`
	FileType        *string   `json:"fileType"`
}

// objectRow is one element of the GET /versions/{versionId}/objects answer.
type objectRow struct {
	ID               int          `json:"id"`
	VersionID        int          `json:"versionId"`
	Name             string       `json:"name"`
	MeshName         string       `json:"meshName"`
	SDKID            string       `json:"sdkId"`
	ScaleCustomX     float64      `json:"scaleCustomX"`
	ScaleCustomY     float64      `json:"scaleCustomY"`
	ScaleCustomZ     float64      `json:"scaleCustomZ"`
	InitialPositionX float64      `json:"initialPositionX"`
	InitialPositionY float64      `json:"initialPositionY"`
	InitialPositionZ float64      `json:"initialPositionZ"`
	InitialRotationX float64      `json:"initialRotationX"`
	InitialRotationY float64      `json:"initialRotationY"`
	InitialRotationZ float64      `json:"initialRotationZ"`
	InitialRotationW float64      `json:"initialRotationW"`
	InitialScaleX    *float64     `json:"initialScaleX"`
	InitialScaleY    *float64     `json:"initialScaleY"`
	InitialScaleZ    *float64     `json:"initialScaleZ"`
	IsController     bool         `json:"isController"`
	Scale            float64      `json:"scale"`
	ExternalID       *string      `json:"externalId"`
	DynamicFileType  *string      `json:"dynamicFileType"`
	Files            []objectFile `json:"files"`
}

type recordedKey struct{}

func withRecorded(r *http.Request, rec Recorded) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), recordedKey{}, rec))
}

func recordedFrom(r *http.Request) Recorded {
	rec, _ := r.Context().Value(recordedKey{}).(Recorded)
	return rec
}

func versionParam(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("version"))
	return n
}

func fileType(name string) string {
	switch ext := strings.ToLower(path.Ext(name)); {
	case name == "cvr_object_thumbnail.png":
		return "thumbnail"
	case ext == ".gltf" || ext == ".glb":
		return "model"
	case ext == ".bin":
		return "buffer"
	default:
		return "texture"
	}
}

// uploadObject records the uploaded file names under the scene, version and
// mesh; a known scene gets 200 with an empty body, an unknown one 401.
func (s *Server) uploadObject(w http.ResponseWriter, r *http.Request) {
	rec := recordedFrom(r)
	s.mu.Lock()
	known := s.findSceneLocked(r.PathValue("sceneId")) != nil
	if known {
		key := meshKey{versionKey{r.PathValue("sceneId"), versionParam(r)}, r.PathValue("meshName")}
		for _, p := range rec.Parts {
			if p.Filename != "" {
				s.objectFiles[key] = append(s.objectFiles[key], objectFile{Name: p.Filename, Type: fileType(p.Filename)})
			}
		}
	}
	s.mu.Unlock()
	if !known {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// uploadManifest upserts one row per manifest entry under the scene and
// version, keyed by sdkId. Malformed JSON and short vectors get 400 "Invalid
// json."; duplicate ids get 400 with an empty body, as the connector's errors
// do; an unknown scene gets 401 with an empty body.
func (s *Server) uploadManifest(w http.ResponseWriter, r *http.Request) {
	sceneID := r.PathValue("sceneId")
	s.mu.Lock()
	known := s.findSceneLocked(sceneID) != nil
	s.mu.Unlock()
	if !known {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var body struct {
		Objects []manifestEntry `json:"objects"`
	}
	if err := json.Unmarshal(recordedFrom(r).Body, &body); err != nil || !vectorsValid(body.Objects) {
		writeText(w, http.StatusBadRequest, "Invalid json.")
		return
	}
	seen := map[string]bool{}
	for _, e := range body.Objects {
		if e.ID == nil || e.Mesh == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		lower := strings.ToLower(*e.ID)
		if seen[lower] {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		seen[lower] = true
	}

	key := versionKey{sceneID, versionParam(r)}
	s.mu.Lock()
	versionID := s.versionIDLocked(key)
	for _, e := range body.Objects {
		row := newRow(e, versionID)
		rows := s.objectRows[key]
		replaced := false
		for i := range rows {
			if strings.EqualFold(rows[i].SDKID, row.SDKID) {
				row.ID = rows[i].ID
				rows[i] = row
				replaced = true
				break
			}
		}
		if !replaced {
			s.nextObjectID++
			row.ID = s.nextObjectID
			s.objectRows[key] = append(rows, row)
		}
	}
	s.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
}

func vectorsValid(entries []manifestEntry) bool {
	for _, e := range entries {
		for _, v := range []struct {
			values []float64
			want   int
		}{{e.ScaleCustom, 3}, {e.InitialPosition, 3}, {e.InitialRotation, 4}, {e.InitialScale, 3}} {
			if v.values != nil && len(v.values) != v.want {
				return false
			}
		}
	}
	return true
}

func vector(values []float64, fallback ...float64) []float64 {
	if values != nil {
		return values
	}
	return fallback
}

func optional(values []float64, i int) *float64 {
	if values == nil {
		return nil
	}
	return &values[i]
}

func newRow(e manifestEntry, versionID int) objectRow {
	name := *e.Mesh
	if e.Name != nil {
		name = *e.Name
	}
	scale, position, rotation := vector(e.ScaleCustom, 1, 1, 1), vector(e.InitialPosition, 0, 0, 0), vector(e.InitialRotation, 0, 0, 0, 1)
	row := objectRow{
		VersionID: versionID, Name: name, MeshName: *e.Mesh, SDKID: *e.ID,
		ScaleCustomX: scale[0], ScaleCustomY: scale[1], ScaleCustomZ: scale[2],
		InitialPositionX: position[0], InitialPositionY: position[1], InitialPositionZ: position[2],
		InitialRotationX: rotation[0], InitialRotationY: rotation[1], InitialRotationZ: rotation[2], InitialRotationW: rotation[3],
		InitialScaleX: optional(e.InitialScale, 0), InitialScaleY: optional(e.InitialScale, 1), InitialScaleZ: optional(e.InitialScale, 2),
		Scale: 1, ExternalID: e.ExternalID, DynamicFileType: e.FileType,
	}
	if e.IsController != nil {
		row.IsController = *e.IsController
	}
	if e.Scale != nil {
		row.Scale = *e.Scale
	}
	return row
}

func (s *Server) versionIDLocked(key versionKey) int {
	if scene := s.findSceneLocked(key.sceneID); scene != nil {
		for _, v := range scene.Versions {
			if v.VersionNumber == key.number {
				return v.ID
			}
		}
	}
	return 0
}

func (s *Server) versionKeyLocked(versionID int) (versionKey, bool) {
	for _, scene := range s.scenes {
		for _, v := range scene.Versions {
			if v.ID == versionID {
				return versionKey{scene.ID, v.VersionNumber}, true
			}
		}
	}
	return versionKey{}, false
}

// listObjects answers one row per manifest entry posted to the version, with
// the files uploaded for its mesh to that same version; an unknown version id
// gets 404 with an empty body.
func (s *Server) listObjects(w http.ResponseWriter, r *http.Request) {
	versionID, err := strconv.Atoi(r.PathValue("versionId"))
	s.mu.Lock()
	key, ok := s.versionKeyLocked(versionID)
	rows := []objectRow{}
	if err == nil && ok {
		for _, row := range s.objectRows[key] {
			row.Files = append([]objectFile{}, s.objectFiles[meshKey{key, row.MeshName}]...)
			rows = append(rows, row)
		}
	}
	s.mu.Unlock()
	if err != nil || !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, rows)
}
