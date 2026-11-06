// Package fakegateway is an in-process stand-in for the Cognitive3D upload
// gateway, for tests only.
package fakegateway

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

const recordedBodyLimitBytes = 1 << 20

// Scene is the gateway's scene object as GET /scenes/{id} returns it.
type Scene struct {
	ID          string    `json:"id"`
	SDKFacingID string    `json:"sdkFacingId"`
	SceneName   string    `json:"sceneName"`
	Versions    []Version `json:"versions"`
}

type Version struct {
	ID            int     `json:"id"`
	VersionNumber int     `json:"versionNumber"`
	SDKVersion    string  `json:"sdkVersion"`
	Scale         float64 `json:"scale"`
}

// User is the body of GET /user; KeyExpiresAt is epoch milliseconds, 0 for
// never.
type User struct {
	ProjectName      string `json:"projectName"`
	OrganizationName string `json:"organizationName"`
	KeyExpiresAt     int64  `json:"keyExpiresAt"`
}

// Recorded is one request as the fake received it. Body and Part.Body are
// kept only when under 1 MB.
type Recorded struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   []byte
	Parts  []RecordedPart
}

type RecordedPart struct {
	Field    string
	Filename string
	Size     int64
	Body     []byte
}

type failure struct {
	status int
	body   string
	header http.Header
}

type Server struct {
	srv        *httptest.Server
	key        string
	expiredKey string

	mu       sync.Mutex
	requests []Recorded
	delays   []time.Duration
	failures []failure
	scenes   []Scene
	user     User
}

type route struct {
	pattern string
	auth    authRule
	handle  http.HandlerFunc
}

func (s *Server) routes() []route {
	return []route{
		{"GET /v0/apiKeys/verify", rejectExpired, s.empty},
		{"GET /v0/user", rejectExpired, s.getUser},
		{"GET /v0/scenes", rejectExpired, s.listScenes},
		{"POST /v0/scenes", rejectExpired, s.empty},
		{"GET /v0/scenes/{sceneId}", acceptExpired, s.getScene},
		{"/", acceptExpired, s.empty},
	}
}

// New starts a fake gateway that is closed when the test ends.
func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{
		key:        newKey("valid"),
		expiredKey: newKey("expired"),
		user:       User{ProjectName: "Fake Project", OrganizationName: "Fake Organization"},
	}
	mux := http.NewServeMux()
	for _, rt := range s.routes() {
		mux.HandleFunc(rt.pattern, s.serve(rt))
	}
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

// URL is the base URL including the /v0 prefix.
func (s *Server) URL() string { return s.srv.URL + "/v0" }

func (s *Server) Key() string { return s.key }

func (s *Server) ExpiredKey() string { return s.expiredKey }

// Requests returns copies of every request received so far, in order.
func (s *Server) Requests() []Recorded {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Recorded, len(s.requests))
	for i, r := range s.requests {
		out[i] = r.clone()
	}
	return out
}

// Delay holds the response to one request for d; each call applies to the
// next request not yet delayed.
func (s *Server) Delay(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delays = append(s.delays, d)
}

// FailWith answers one request with status and a plain-text body, before
// authentication; each call applies to the next request not yet failed.
func (s *Server) FailWith(status int, body string) { s.FailWithHeaders(status, body, nil) }

// FailWithHeaders is FailWith with extra response headers.
func (s *Server) FailWithHeaders(status int, body string, header http.Header) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = append(s.failures, failure{status: status, body: body, header: header.Clone()})
}

// AddScene makes the scene visible to GET /scenes and GET /scenes/{id}.
func (s *Server) AddScene(scene Scene) {
	if scene.Versions == nil {
		scene.Versions = []Version{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scenes = append(s.scenes, scene)
}

func (s *Server) SetUser(u User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.user = u
}

func (s *Server) serve(rt route) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.record(r)
		delay, fail := s.next()
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-r.Context().Done():
				return
			}
		}
		if fail != nil {
			for name, values := range fail.header {
				w.Header()[name] = values
			}
			writeText(w, fail.status, fail.body)
			return
		}
		if !s.authenticate(w, r, rt.auth) {
			return
		}
		rt.handle(w, r)
	}
}

func (s *Server) next() (time.Duration, *failure) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var delay time.Duration
	var fail *failure
	if len(s.delays) > 0 {
		delay, s.delays = s.delays[0], s.delays[1:]
	}
	if len(s.failures) > 0 {
		fail = &s.failures[0]
		s.failures = s.failures[1:]
	}
	return delay, fail
}

func (s *Server) record(r *http.Request) {
	rec := Recorded{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Header: r.Header.Clone()}
	if mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mediaType == "multipart/form-data" {
		rec.Parts = readParts(r)
		// Reading to EOF lets the server notice when the client hangs up.
		_, _ = io.Copy(io.Discard, r.Body)
	} else {
		rec.Body, _ = readCapped(r.Body)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, rec)
}

func readParts(r *http.Request) []RecordedPart {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil
	}
	var parts []RecordedPart
	for {
		p, err := mr.NextRawPart()
		if err != nil {
			return parts
		}
		_, params, _ := mime.ParseMediaType(p.Header.Get("Content-Disposition"))
		body, size := readCapped(p)
		parts = append(parts, RecordedPart{Field: params["name"], Filename: params["filename"], Size: size, Body: body})
	}
}

// readCapped drains r and returns its bytes when under the record limit.
func readCapped(r io.Reader) ([]byte, int64) {
	var buf bytes.Buffer
	n, _ := io.Copy(&buf, io.LimitReader(r, recordedBodyLimitBytes))
	rest, _ := io.Copy(io.Discard, r)
	if rest > 0 || n >= recordedBodyLimitBytes {
		return nil, n + rest
	}
	return buf.Bytes(), n
}

func (r Recorded) clone() Recorded {
	out := r
	out.Query = cloneValues(r.Query)
	out.Header = r.Header.Clone()
	out.Body = bytes.Clone(r.Body)
	if r.Parts != nil {
		out.Parts = make([]RecordedPart, len(r.Parts))
		for i, p := range r.Parts {
			p.Body = bytes.Clone(p.Body)
			out.Parts[i] = p
		}
	}
	return out
}

func cloneValues(v url.Values) url.Values {
	out := make(url.Values, len(v))
	for k, vs := range v {
		out[k] = append([]string(nil), vs...)
	}
	return out
}

func writeText(w http.ResponseWriter, status int, body string) {
	if body != "" && w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func writeJSON(w http.ResponseWriter, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		writeText(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *Server) empty(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

func (s *Server) getUser(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	u := s.user
	s.mu.Unlock()
	writeJSON(w, u)
}

func (s *Server) listScenes(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	scenes := append([]Scene{}, s.scenes...)
	s.mu.Unlock()
	writeJSON(w, scenes)
}

// getScene answers an unknown id with 401 and an empty body, as the gateway
// does for an id outside the key's project.
func (s *Server) getScene(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("sceneId")
	s.mu.Lock()
	var found *Scene
	for i := range s.scenes {
		if s.scenes[i].ID == id {
			scene := s.scenes[i]
			found = &scene
			break
		}
	}
	s.mu.Unlock()
	if found == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	writeJSON(w, found)
}
