package model

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Severity int

const (
	Warning Severity = iota + 1
	Error
)

// Finding is one problem Check found in a model. Errors block the upload.
type Finding struct {
	Severity Severity
	Message  string
}

const (
	glbMagic        = 0x46546C67
	glbChunkJSON    = 0x4E4F534A
	glbHeaderBytes  = 12
	glbChunkHeader  = 8
	maxGLBJSONBytes = 8 << 20
)

type gltfDocument struct {
	Buffers []gltfBuffer `json:"buffers"`
	Images  []gltfImage  `json:"images"`
}

type gltfBuffer struct {
	URI        string `json:"uri"`
	ByteLength *int64 `json:"byteLength"`
}

type gltfImage struct {
	URI string `json:"uri"`
}

// Check inspects the model in files against the base file names that will be
// uploaded (model files plus kept textures). The returned error covers only
// I/O failures reading the model; everything else is a Finding.
func Check(files Files, dir string, uploaded []string, mode Mode) ([]Finding, error) {
	if len(files.Paths) == 0 {
		return nil, fmt.Errorf("no model files to check")
	}
	if files.Kind == GLB {
		return checkGLB(files.Paths[0])
	}
	return checkGLTF(files.Paths[0], dir, uploaded, mode)
}

func errorf(format string, args ...any) Finding {
	return Finding{Severity: Error, Message: fmt.Sprintf(format, args...)}
}

func checkGLTF(path, dir string, uploaded []string, mode Mode) ([]Finding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc gltfDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return []Finding{errorf("%s is not a valid glTF JSON file: %v", filepath.Base(path), err)}, nil
	}

	var findings []Finding
	reported := map[string]bool{}
	for i, b := range doc.Buffers {
		if b.URI == "" {
			continue
		}
		name, f := resolveURI(fmt.Sprintf("buffers[%d]", i), b.URI, true, dir, uploaded, mode)
		if f != nil {
			if !reported[b.URI] {
				reported[b.URI] = true
				findings = append(findings, *f)
			}
			continue
		}
		if b.ByteLength == nil {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			return findings, err
		}
		if info.Size() != *b.ByteLength {
			findings = append(findings, errorf("buffers[%d] declares byteLength %d bytes but %s is %d bytes", i, *b.ByteLength, name, info.Size()))
		}
	}
	for i, img := range doc.Images {
		if img.URI == "" {
			continue
		}
		if _, f := resolveURI(fmt.Sprintf("images[%d]", i), img.URI, false, dir, uploaded, mode); f != nil && !reported[img.URI] {
			reported[img.URI] = true
			findings = append(findings, *f)
		}
	}
	return findings, nil
}

// resolveURI returns the uploaded file name a uri refers to, or the one
// finding that explains why it does not refer to one.
func resolveURI(where, raw string, isBuffer bool, dir string, uploaded []string, mode Mode) (string, *Finding) {
	fail := func(format string, args ...any) (string, *Finding) {
		f := errorf("%s uri %q: %s", where, raw, fmt.Sprintf(format, args...))
		return "", &f
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "data:") {
		return "", &Finding{Severity: Warning, Message: fmt.Sprintf("%s holds an embedded data: uri; it is uploaded inside the .gltf", where)}
	}
	if strings.Contains(raw, "://") || strings.HasPrefix(lower, "http:") || strings.HasPrefix(lower, "https:") {
		return fail("remote uri; the gateway does not fetch remote files, so the file must be in the upload directory")
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return fail("cannot be percent-decoded: %v", err)
	}
	name := strings.TrimPrefix(decoded, "./")
	if name == ".." || strings.HasPrefix(name, "../") || strings.HasPrefix(name, `..\`) {
		return fail("refers to the parent directory, which is not uploaded; move the file next to the .gltf")
	}
	if strings.ContainsAny(name, `/\`) {
		return fail("refers to a subdirectory; subdirectories are not uploaded, so move the file next to the .gltf")
	}
	if name == "" || name == "." {
		return fail("does not name a file")
	}
	if isBuffer && mode == SceneMode && name != "scene.bin" {
		return fail("the gateway stores every .bin as scene.bin, so a scene's buffer must be named scene.bin")
	}

	for _, u := range uploaded {
		if u == name {
			return name, nil
		}
	}
	for _, u := range uploaded {
		if strings.EqualFold(u, name) {
			return fail("the file is uploaded as %q; the case differs and the gateway matches names exactly", u)
		}
	}
	if inDirectory(dir, name) {
		if isBuffer {
			return fail("%s is in the directory but is not part of the upload", name)
		}
		return fail("%s is not an accepted texture type (.png .jpg .jpeg .webp) so it is skipped, but the model needs it", name)
	}
	return fail("%s is missing from the directory", name)
}

func inDirectory(dir, name string) bool {
	names, err := listFiles(dir)
	if err != nil {
		return false
	}
	for _, n := range names {
		if strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

func checkGLB(path string) ([]Finding, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	base := filepath.Base(path)

	var header [glbHeaderBytes + glbChunkHeader]byte
	n, err := io.ReadFull(f, header[:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if n < glbHeaderBytes {
		return []Finding{errorf("%s is truncated: %d bytes is shorter than the 12-byte .glb header", base, size)}, nil
	}

	var findings []Finding
	if binary.LittleEndian.Uint32(header[0:4]) != glbMagic {
		return []Finding{errorf("%s is not a .glb file: it does not start with the glTF magic bytes", base)}, nil
	}
	if v := binary.LittleEndian.Uint32(header[4:8]); v != 2 {
		return []Finding{errorf("%s is .glb version %d; only version 2 is supported", base, v)}, nil
	}
	if total := binary.LittleEndian.Uint32(header[8:12]); int64(total) != size {
		findings = append(findings, errorf("%s declares a length of %d bytes but the file is %d bytes", base, total, size))
	}

	if n < glbHeaderBytes+glbChunkHeader {
		return append(findings, errorf("%s is truncated: it has no JSON chunk", base)), nil
	}
	chunkLength := binary.LittleEndian.Uint32(header[12:16])
	if binary.LittleEndian.Uint32(header[16:20]) != glbChunkJSON {
		return append(findings, errorf("%s: the first chunk is not JSON", base)), nil
	}
	if chunkLength > maxGLBJSONBytes {
		return append(findings, errorf("%s: JSON chunk is %d bytes, over the 8 MB limit", base, chunkLength)), nil
	}
	if int64(glbHeaderBytes+glbChunkHeader)+int64(chunkLength) > size {
		return append(findings, errorf("%s is truncated: the JSON chunk extends past the end of the file", base)), nil
	}

	body := make([]byte, chunkLength)
	if _, err := f.ReadAt(body, glbHeaderBytes+glbChunkHeader); err != nil {
		return findings, err
	}
	var doc gltfDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return append(findings, errorf("%s: the JSON chunk is not valid glTF JSON: %v", base, err)), nil
	}
	for i, b := range doc.Buffers {
		if b.URI != "" {
			findings = append(findings, errorf("buffers[%d] has external uri %q; a .glb must be self-contained", i, b.URI))
		}
	}
	for i, img := range doc.Images {
		if img.URI != "" {
			findings = append(findings, errorf("images[%d] has external uri %q; a .glb must be self-contained", i, img.URI))
		}
	}
	return findings, nil
}
