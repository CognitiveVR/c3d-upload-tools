// Package gateway is the HTTP client for the Cognitive3D upload gateway.
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cognitivevr/c3d-upload-tools/internal/ui"
	"github.com/cognitivevr/c3d-upload-tools/internal/version"
)

const (
	defaultResponseHeaderTimeout = 60 * time.Second
	dialTimeout                  = 30 * time.Second
	tlsHandshakeTimeout          = 30 * time.Second
	debugBodyLimitBytes          = 4096
)

// Part is one multipart part. Open is called once per request and may return
// an in-memory reader.
type Part struct {
	Field       string
	Filename    string
	ContentType string
	Size        int64
	Open        func() (io.ReadCloser, error)
}

type Response struct {
	StatusCode  int
	Body        []byte
	ContentType string

	header http.Header
}

// User is the body of GET /user. KeyExpiresAtMilliseconds is epoch
// milliseconds, 0 for a key that never expires.
type User struct {
	ProjectName              string `json:"projectName"`
	OrganizationName         string `json:"organizationName"`
	KeyExpiresAtMilliseconds int64  `json:"keyExpiresAt"`
}

// Client is safe for concurrent use once its first request has started.
type Client struct {
	// ResponseHeaderTimeout bounds the wait for response headers on every
	// request except PostMultipart. It is read when the first such request is
	// made.
	ResponseHeaderTimeout time.Duration

	baseURL   string
	host      string
	apiKey    string
	userAgent string
	log       *ui.Logger

	preflighted atomic.Bool

	apiOnce    sync.Once
	apiHTTP    *http.Client
	uploadOnce sync.Once
	uploadHTTP *http.Client

	sleep  func(ctx context.Context, d time.Duration) error
	random func() float64
}

// UserAgent is the User-Agent this binary sends.
func UserAgent() string {
	return fmt.Sprintf("%s/%s (%s/%s)", binaryName, version.Version, runtime.GOOS, runtime.GOARCH)
}

// New returns a client for baseURL, which includes the /v0 prefix. A nil log
// discards debug output.
func New(baseURL, apiKey, userAgent string, log *ui.Logger) *Client {
	if log == nil {
		log = ui.New(io.Discard, ui.Options{})
	}
	host := baseURL
	if u, err := url.Parse(baseURL); err == nil && u.Host != "" {
		host = u.Host
	}
	return &Client{
		ResponseHeaderTimeout: defaultResponseHeaderTimeout,
		baseURL:               strings.TrimRight(baseURL, "/"),
		host:                  host,
		apiKey:                apiKey,
		userAgent:             userAgent,
		log:                   log,
		sleep:                 sleepContext,
		random:                rand.Float64,
	}
}

// MarkPreflighted records that the key was verified, so a later 401 with an
// empty body on a scene-scoped route means the scene is not accessible.
func (c *Client) MarkPreflighted() { c.preflighted.Store(true) }

func (c *Client) VerifyKey(ctx context.Context) error {
	return c.Get(ctx, "/apiKeys/verify", nil, nil)
}

func (c *Client) User(ctx context.Context) (User, error) {
	var u User
	err := c.Get(ctx, "/user", nil, &u)
	return u, err
}

// Get retries 502, 503, 504 and transport errors; out may be nil.
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) error {
	resp, err := c.withRetry(ctx, path, func() (*Response, error) {
		return c.attempt(ctx, c.apiClient(), http.MethodGet, path, query, nil, 0, "")
	})
	if err != nil {
		return err
	}
	return c.decode(http.MethodGet, path, resp, out)
}

func (c *Client) Delete(ctx context.Context, path string, query url.Values, out any) error {
	resp, err := c.attempt(ctx, c.apiClient(), http.MethodDelete, path, query, nil, 0, "")
	if err != nil {
		return err
	}
	return c.decode(http.MethodDelete, path, resp, out)
}

func (c *Client) PostJSON(ctx context.Context, path string, query url.Values, body any) (*Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode POST %s body: %w", path, err)
	}
	reader := &sentReader{r: bytes.NewReader(data), remaining: int64(len(data))}
	resp, err := c.attempt(ctx, c.apiClient(), http.MethodPost, path, query, reader, int64(len(data)), "application/json")
	return c.result(http.MethodPost, path, resp, err, reader.sent.Load())
}

// PostMultipart has no response-header timeout: the gateway answers only
// after it has processed every part.
func (c *Client) PostMultipart(ctx context.Context, path string, query url.Values, parts []Part, progress *ui.Progress) (*Response, error) {
	body := buildMultipartBody(parts, progress)
	resp, err := c.attempt(ctx, c.uploadClient(), http.MethodPost, path, query, body, body.contentLength, body.contentType)
	_ = body.Close()
	<-body.done
	if body.err != nil {
		return nil, body.err
	}
	return c.result(http.MethodPost, path, resp, err, body.sent.Load())
}

func (c *Client) result(method, path string, resp *Response, err error, bodySent bool) (*Response, error) {
	if err == nil {
		err = c.statusError(method, path, resp)
	}
	if err != nil {
		var gerr *Error
		if errors.As(err, &gerr) {
			gerr.BodySent = bodySent
		}
		return nil, err
	}
	return resp, nil
}

func (c *Client) decode(method, path string, resp *Response, out any) error {
	if err := c.statusError(method, path, resp); err != nil {
		return err
	}
	if out == nil || len(bytes.TrimSpace(resp.Body)) == 0 {
		return nil
	}
	if err := json.Unmarshal(resp.Body, out); err != nil {
		return fmt.Errorf("decode %s %s response: %w", method, path, err)
	}
	return nil
}

func (c *Client) statusError(method, path string, resp *Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	shown := resp.Body
	if len(shown) > debugBodyLimitBytes {
		shown = shown[:debugBodyLimitBytes]
	}
	c.log.Debug("%s %s returned status %d: %s", method, path, resp.StatusCode, shown)
	return newStatusError(method, path, resp.StatusCode, resp.Body, resp.ContentType, c.preflighted.Load())
}

// attempt sends one request and reads the whole response, whatever its
// status. Transport and read failures come back as *Error. contentLength -1
// means unknown (chunked).
func (c *Client) attempt(ctx context.Context, hc *http.Client, method, path string, query url.Values, body io.Reader, contentLength int64, contentType string) (*Response, error) {
	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, fmt.Errorf("build %s %s request: %w", method, path, err)
	}
	if body != nil {
		req.ContentLength = contentLength
	}
	req.Header.Set("Authorization", "APIKEY:DEVELOPER "+c.apiKey)
	req.Header.Set("User-Agent", c.userAgent)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if method == http.MethodGet || method == http.MethodDelete {
		req.Header.Set("Accept", "application/json")
	}
	c.log.Debug("%s %s", method, path)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, c.transportError(ctx, method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, c.transportError(ctx, method, path, err)
	}
	return &Response{StatusCode: resp.StatusCode, Body: data, ContentType: resp.Header.Get("Content-Type"), header: resp.Header}, nil
}

func (c *Client) transportError(ctx context.Context, method, path string, err error) *Error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		err = uerr.Err
	}
	return newTransportError(method, path, c.host, err, false, ctx.Err() != nil)
}

func (c *Client) apiClient() *http.Client {
	c.apiOnce.Do(func() { c.apiHTTP = newHTTPClient(c.ResponseHeaderTimeout) })
	return c.apiHTTP
}

func (c *Client) uploadClient() *http.Client {
	c.uploadOnce.Do(func() { c.uploadHTTP = newHTTPClient(0) })
	return c.uploadHTTP
}

// newHTTPClient has no overall timeout; a zero responseHeaderTimeout waits
// for headers indefinitely. Redirects are returned, not followed.
func newHTTPClient(responseHeaderTimeout time.Duration) *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = http.ProxyFromEnvironment
	t.DialContext = (&net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}).DialContext
	t.TLSHandshakeTimeout = tlsHandshakeTimeout
	t.ResponseHeaderTimeout = responseHeaderTimeout
	return &http.Client{
		Transport:     t,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// sentReader reports whether every byte of a body of known size was read.
type sentReader struct {
	r         io.Reader
	remaining int64
	sent      atomic.Bool
}

func (s *sentReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	s.remaining -= int64(n)
	if s.remaining <= 0 {
		s.sent.Store(true)
	}
	return n, err
}

// multipartBody streams parts to the transport. sent becomes true once the
// closing boundary has been consumed; err holds a local failure to read a
// part and is valid after done is closed.
type multipartBody struct {
	*io.PipeReader
	contentType   string
	contentLength int64
	sent          atomic.Bool
	done          chan struct{}
	err           error
}

func buildMultipartBody(parts []Part, progress *ui.Progress) *multipartBody {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	body := &multipartBody{PipeReader: pr, contentType: mw.FormDataContentType(), contentLength: -1, done: make(chan struct{})}
	go func() {
		defer close(body.done)
		err := writeParts(mw, parts, progress)
		var local *partError
		if errors.As(err, &local) {
			body.err = local.err
		}
		if err == nil {
			if err = mw.Close(); err == nil {
				body.sent.Store(true)
			}
		}
		_ = pw.CloseWithError(err)
	}()
	return body
}

type partError struct{ err error }

func (e *partError) Error() string { return e.err.Error() }

func writeParts(mw *multipart.Writer, parts []Part, progress *ui.Progress) error {
	for _, part := range parts {
		if err := writePart(mw, part, progress); err != nil {
			return err
		}
	}
	return nil
}

func writePart(mw *multipart.Writer, part Part, progress *ui.Progress) error {
	contentType := part.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", multipart.FileContentDisposition(part.Field, part.Filename))
	header.Set("Content-Type", contentType)
	w, err := mw.CreatePart(header)
	if err != nil {
		return err
	}
	rc, err := part.Open()
	if err != nil {
		return &partError{err: fmt.Errorf("open %s: %w", part.Filename, err)}
	}
	defer func() { _ = rc.Close() }()
	src := &readErrorRecorder{r: rc}
	var r io.Reader = src
	if progress != nil {
		r = ui.CountingReader(src, progress)
	}
	if _, err := io.Copy(w, r); err != nil {
		if src.err != nil {
			return &partError{err: fmt.Errorf("read %s: %w", part.Filename, src.err)}
		}
		return err
	}
	return nil
}

// readErrorRecorder separates a failure reading a part from a failure writing
// it to a transport that has stopped reading.
type readErrorRecorder struct {
	r   io.Reader
	err error
}

func (r *readErrorRecorder) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		r.err = err
	}
	return n, err
}
