package fakegateway

import (
	"encoding/json"
	"net/http"
	"sort"
)

// egoSlot is one element of the GET /scenes/{sceneId}/egos answer. Manifest
// is the ego.json body when one was sent and valid JSON, else null.
type egoSlot struct {
	Key       string          `json:"key"`
	Model     string          `json:"model"`
	Files     []string        `json:"files"`
	Manifest  json.RawMessage `json:"manifest"`
	Optimized bool            `json:"optimized"`
}

type slotKey struct {
	versionKey
	slot string
}

// SetOptimizedAfterPolls makes the n-th GET /scenes/{sceneId}/egos for the
// version, counted while the slot holds a model, answer it as optimized.
func (s *Server) SetOptimizedAfterPolls(sceneID string, version int, slot string, polls int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.optimizeAfter[slotKey{versionKey{sceneID, version}, slot}] = polls
}

func (s *Server) knownScene(r *http.Request) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.findSceneLocked(r.PathValue("sceneId")) != nil
}

// uploadEgo replaces the slot with the uploaded parts and answers 200 with
// the slot; an unknown scene gets 401 with an empty body.
func (s *Server) uploadEgo(w http.ResponseWriter, r *http.Request) {
	if !s.knownScene(r) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	slot := &egoSlot{Key: r.PathValue("key"), Files: []string{}, Manifest: json.RawMessage("null")}
	for _, p := range recordedFrom(r).Parts {
		if p.Filename == "" {
			continue
		}
		slot.Files = append(slot.Files, p.Filename)
		ext := fileType(p.Filename)
		if slot.Model == "" && (ext == "glb" || ext == "gltf") {
			slot.Model = p.Filename
		}
		if p.Filename == "ego.json" && json.Valid(p.Body) {
			slot.Manifest = json.RawMessage(p.Body)
		}
	}
	key := versionKey{r.PathValue("sceneId"), versionParam(r)}
	s.mu.Lock()
	if s.egos[key] == nil {
		s.egos[key] = map[string]*egoSlot{}
	}
	s.egos[key][slot.Key] = slot
	answer := *slot
	s.mu.Unlock()
	writeJSON(w, answer)
}

// listEgos answers the version's slots sorted by key; an unknown scene gets
// 401 with an empty body.
func (s *Server) listEgos(w http.ResponseWriter, r *http.Request) {
	if !s.knownScene(r) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	key := versionKey{r.PathValue("sceneId"), versionParam(r)}
	s.mu.Lock()
	egos := []egoSlot{}
	for name, slot := range s.egos[key] {
		hook := slotKey{key, name}
		if polls, ok := s.optimizeAfter[hook]; ok {
			if polls <= 1 {
				slot.Optimized = true
				delete(s.optimizeAfter, hook)
			} else {
				s.optimizeAfter[hook] = polls - 1
			}
		}
		copied := *slot
		copied.Files = append([]string{}, slot.Files...)
		egos = append(egos, copied)
	}
	s.mu.Unlock()
	sort.Slice(egos, func(i, j int) bool { return egos[i].Key < egos[j].Key })
	writeJSON(w, struct {
		SceneID string    `json:"sceneId"`
		Version int       `json:"version"`
		Egos    []egoSlot `json:"egos"`
	}{key.sceneID, key.number, egos})
}

// deleteEgo empties the slot and answers 200 {"deleted":true} whether or not
// it held a model; an unknown scene gets 401 with an empty body.
func (s *Server) deleteEgo(w http.ResponseWriter, r *http.Request) {
	if !s.knownScene(r) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	key := versionKey{r.PathValue("sceneId"), versionParam(r)}
	s.mu.Lock()
	delete(s.egos[key], r.PathValue("key"))
	s.mu.Unlock()
	writeJSON(w, map[string]bool{"deleted": true})
}
