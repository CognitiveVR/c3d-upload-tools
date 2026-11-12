// Package fakegateway is an in-process stand-in for the Cognitive3D upload
// gateway, for tests only.
package fakegateway

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
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
// kept only when under 1 MB. Complete is false when the body ended early.
// ContentLength is -1 for a body of undeclared length; BodyBytes counts every
// body byte read, kept or not.
type Recorded struct {
	Method           string
	Path             string
	RawPath          string
	Query            url.Values
	Header           http.Header
	ContentLength    int64
	TransferEncoding []string
	Body             []byte
	BodyBytes        int64
	Parts            []RecordedPart
	Complete         bool
}

type RecordedPart struct {
	Field       string
	Filename    string
	ContentType string
	Size        int64
	Body        []byte
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

	mu            sync.Mutex
	requests      []Recorded
	delays        []time.Duration
	failures      []failure
	scenes        []Scene
	user          User
	nextVersionID int
	planned       map[string]pendingVersions
	hidden        map[string]*pendingVersions
	objectFiles   map[meshKey][]objectFile
	objectRows    map[versionKey][]objectRow
	nextObjectID  int
	egos          map[versionKey]map[string]*egoSlot
	optimizeAfter map[slotKey]int
	routeFailures map[string]failure
	routeHooks    map[string][]func()
}

// pendingVersions are versions a POST adds, left out of the next polls GET
// answers for the scene.
type pendingVersions struct {
	polls    int
	versions []Version
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
		{"POST /v0/scenes", rejectExpired, s.createScene},
		{"GET /v0/scenes/{sceneId}", acceptExpired, s.getScene},
		{"POST /v0/scenes/{sceneId}", acceptExpired, s.addVersion},
		{"POST /v0/scenes/{sceneId}/screenshot", acceptExpired, s.replaceScreenshot},
		{"GET /v0/scenes/{sceneId}/egos", acceptExpired, s.listEgos},
		{"POST /v0/scenes/{sceneId}/egos/{key}", acceptExpired, s.uploadEgo},
		{"DELETE /v0/scenes/{sceneId}/egos/{key}", acceptExpired, s.deleteEgo},
		{"POST /v0/objects/{sceneId}/{meshName}", acceptExpired, s.uploadObject},
		{"POST /v0/objects/{sceneId}", acceptExpired, s.uploadManifest},
		{"GET /v0/versions/{versionId}/objects", acceptExpired, s.listObjects},
		{"/", acceptExpired, s.empty},
	}
}

// New starts a fake gateway that is closed when the test ends.
func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{
		key:           newKey("valid"),
		expiredKey:    newKey("expired"),
		user:          User{ProjectName: "Fake Project", OrganizationName: "Fake Organization"},
		nextVersionID: 1000,
		planned:       map[string]pendingVersions{},
		hidden:        map[string]*pendingVersions{},
		objectFiles:   map[meshKey][]objectFile{},
		objectRows:    map[versionKey][]objectRow{},
		egos:          map[versionKey]map[string]*egoSlot{},
		optimizeAfter: map[slotKey]int{},
		routeFailures: map[string]failure{},
		routeHooks:    map[string][]func(){},
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

// FailRoute answers every later authenticated request to the route pattern,
// spelled as in routes (such as "GET /v0/versions/{versionId}/objects"), with
// status, a plain-text body and header.
func (s *Server) FailRoute(pattern string, status int, body string, header http.Header) {
	s.mustBeRoute(pattern)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routeFailures[pattern] = failure{status: status, body: body, header: header.Clone()}
}

// OnRoute runs fn before each later authenticated request to the route
// pattern is answered.
func (s *Server) OnRoute(pattern string, fn func()) {
	s.mustBeRoute(pattern)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routeHooks[pattern] = append(s.routeHooks[pattern], fn)
}

func (s *Server) mustBeRoute(pattern string) {
	for _, rt := range s.routes() {
		if rt.pattern == pattern {
			return
		}
	}
	panic("fakegateway: no route " + pattern)
}

func (s *Server) routeState(pattern string) ([]func(), *failure) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hooks := append([]func(){}, s.routeHooks[pattern]...)
	if f, ok := s.routeFailures[pattern]; ok {
		return hooks, &f
	}
	return hooks, nil
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

// AddVersion adds v to the known scene sceneID, visible at once, as if
// another client had uploaded it.
func (s *Server) AddVersion(sceneID string, v Version) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scene := s.findSceneLocked(sceneID)
	if scene == nil {
		panic("fakegateway: no scene " + sceneID)
	}
	scene.Versions = append(scene.Versions, v)
}

// SetNextUpload decides what the next POST /scenes/{sceneId} for sceneID
// adds: versions, left out of the first hiddenPolls GET /scenes/{sceneId}
// answers after that POST. Without it a POST adds the next version number,
// visible at once.
func (s *Server) SetNextUpload(sceneID string, hiddenPolls int, versions ...Version) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.planned[sceneID] = pendingVersions{polls: hiddenPolls, versions: append([]Version(nil), versions...)}
}

func (s *Server) SetUser(u User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.user = u
}

func (s *Server) serve(rt route) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r = withRecorded(r, s.record(r))
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
			writeFailure(w, fail)
			return
		}
		if !s.authenticate(w, r, rt.auth) {
			return
		}
		hooks, routeFail := s.routeState(rt.pattern)
		for _, hook := range hooks {
			hook()
		}
		if routeFail != nil {
			writeFailure(w, routeFail)
			return
		}
		rt.handle(w, r)
	}
}

func writeFailure(w http.ResponseWriter, f *failure) {
	for name, values := range f.header {
		w.Header()[name] = values
	}
	writeText(w, f.status, f.body)
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

func (s *Server) record(r *http.Request) Recorded {
	rec := Recorded{
		Method: r.Method, Path: r.URL.Path, RawPath: r.URL.EscapedPath(), Query: r.URL.Query(), Header: r.Header.Clone(),
		ContentLength: r.ContentLength, TransferEncoding: append([]string(nil), r.TransferEncoding...),
	}
	body := &countingBody{ReadCloser: r.Body}
	r.Body = body
	var err error
	if mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mediaType == "multipart/form-data" {
		rec.Parts, err = readParts(r)
		// Reading to EOF lets the server notice when the client hangs up.
		if _, drainErr := io.Copy(io.Discard, r.Body); err == nil {
			err = drainErr
		}
	} else {
		rec.Body, _, err = readCapped(r.Body)
	}
	rec.Complete = err == nil
	rec.BodyBytes = body.bytes
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, rec)
	return rec
}

type countingBody struct {
	io.ReadCloser
	bytes int64
}

func (c *countingBody) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	c.bytes += int64(n)
	return n, err
}

func readParts(r *http.Request) ([]RecordedPart, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, err
	}
	var parts []RecordedPart
	for {
		p, err := mr.NextRawPart()
		if errors.Is(err, io.EOF) {
			return parts, nil
		}
		if err != nil {
			return parts, err
		}
		_, params, _ := mime.ParseMediaType(p.Header.Get("Content-Disposition"))
		body, size, err := readCapped(p)
		parts = append(parts, RecordedPart{
			Field: params["name"], Filename: params["filename"], ContentType: p.Header.Get("Content-Type"), Size: size, Body: body,
		})
		if err != nil {
			return parts, err
		}
	}
}

// readCapped drains r and returns its bytes when under the record limit.
func readCapped(r io.Reader) ([]byte, int64, error) {
	var buf bytes.Buffer
	n, err := io.Copy(&buf, io.LimitReader(r, recordedBodyLimitBytes))
	if err != nil {
		return nil, n, err
	}
	rest, err := io.Copy(io.Discard, r)
	if rest > 0 || n >= recordedBodyLimitBytes {
		return nil, n + rest, err
	}
	return buf.Bytes(), n, err
}

func (r Recorded) clone() Recorded {
	out := r
	out.Query = cloneValues(r.Query)
	out.Header = r.Header.Clone()
	out.TransferEncoding = append([]string(nil), r.TransferEncoding...)
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
	if scene := s.findSceneLocked(id); scene != nil {
		if pending := s.hidden[id]; pending != nil {
			if pending.polls > 0 {
				pending.polls--
			} else {
				scene.Versions = append(scene.Versions, pending.versions...)
				delete(s.hidden, id)
			}
		}
		copied := *scene
		copied.Versions = slices.Clone(scene.Versions)
		found = &copied
	}
	s.mu.Unlock()
	if found == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	writeJSON(w, found)
}

// createScene answers 201 with the new scene id as plain text and registers
// the scene with version 1.
func (s *Server) createScene(w http.ResponseWriter, _ *http.Request) {
	id := newSceneID()
	s.mu.Lock()
	s.scenes = append(s.scenes, Scene{ID: id, SDKFacingID: id, Versions: []Version{s.newVersionLocked(1)}})
	s.mu.Unlock()
	writeText(w, http.StatusCreated, id)
}

// addVersion answers 201 with an empty body; an unknown scene gets 401 with
// an empty body, as GET does.
func (s *Server) addVersion(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("sceneId")
	s.mu.Lock()
	scene := s.findSceneLocked(id)
	if scene != nil {
		pending, ok := s.planned[id]
		delete(s.planned, id)
		if !ok {
			latest := 0
			for _, v := range scene.Versions {
				latest = max(latest, v.VersionNumber)
			}
			pending.versions = []Version{s.newVersionLocked(latest + 1)}
		}
		if pending.polls > 0 {
			s.hidden[id] = &pending
		} else {
			scene.Versions = append(scene.Versions, pending.versions...)
		}
	}
	s.mu.Unlock()
	if scene == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// replaceScreenshot answers 200 with an empty body for a known scene and 401
// with an empty body for an unknown one.
func (s *Server) replaceScreenshot(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	known := s.findSceneLocked(r.PathValue("sceneId")) != nil
	s.mu.Unlock()
	if !known {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) findSceneLocked(id string) *Scene {
	for i := range s.scenes {
		if s.scenes[i].ID == id {
			return &s.scenes[i]
		}
	}
	return nil
}

func (s *Server) newVersionLocked(number int) Version {
	s.nextVersionID++
	return Version{ID: s.nextVersionID, VersionNumber: number, Scale: 1}
}

// newSceneID returns a random lowercase UUID v4.
func newSceneID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}
