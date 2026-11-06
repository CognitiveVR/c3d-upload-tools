package fakegateway

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

type authRule int

const (
	// acceptExpired matches the gateway's scene-scoped routes, which check
	// only that the key exists.
	acceptExpired authRule = iota
	rejectExpired
)

const authPrefix = "APIKEY:DEVELOPER "

func newKey(kind string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "fake-" + kind + "-" + hex.EncodeToString(b)
}

// authenticate writes the gateway's 401 and returns false when the request's
// key does not satisfy rule.
func (s *Server) authenticate(w http.ResponseWriter, r *http.Request, rule authRule) bool {
	value := r.Header.Get("Authorization")
	if value == "" {
		writeText(w, http.StatusUnauthorized, "Missing API key.")
		return false
	}
	key, ok := strings.CutPrefix(value, authPrefix)
	if !ok || (key != s.key && key != s.expiredKey) {
		writeText(w, http.StatusUnauthorized, "Nonexistent API key.")
		return false
	}
	if key == s.expiredKey && rule == rejectExpired {
		writeText(w, http.StatusUnauthorized, "Key expired.")
		return false
	}
	return true
}
