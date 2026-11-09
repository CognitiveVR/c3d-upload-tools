package object

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/gateway"
)

// List reads the objects of one scene version and returns the gateway's JSON
// array unchanged.
func List(ctx context.Context, client *gateway.Client, versionID int64) ([]byte, error) {
	var raw json.RawMessage
	if err := client.Get(ctx, "/versions/"+strconv.FormatInt(versionID, 10)+"/objects", nil, &raw); err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
		return nil, errors.New("the gateway's object list is not a JSON array")
	}
	return raw, nil
}

// Indent re-indents a JSON array with two spaces and a trailing newline.
func Indent(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return nil, fmt.Errorf("format object list: %w", err)
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// Listed identifies one listed object.
type Listed struct {
	SDKID    string
	MeshName string
}

type listedRow struct {
	SDKID    string            `json:"sdkId"`
	MeshName string            `json:"meshName"`
	Files    []json.RawMessage `json:"files"`
}

func parseRows(raw []byte) ([]listedRow, error) {
	var rows []listedRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("parse object list: %w", err)
	}
	return rows, nil
}

// All returns every listed object, in list order.
func All(raw []byte) ([]Listed, error) {
	rows, err := parseRows(raw)
	if err != nil {
		return nil, err
	}
	out := make([]Listed, 0, len(rows))
	for _, r := range rows {
		out = append(out, Listed{SDKID: r.SDKID, MeshName: r.MeshName})
	}
	return out, nil
}

// WithoutFiles returns the listed objects whose files array is empty, in
// list order.
func WithoutFiles(raw []byte) ([]Listed, error) {
	rows, err := parseRows(raw)
	if err != nil {
		return nil, err
	}
	var out []Listed
	for _, r := range rows {
		if len(r.Files) == 0 {
			out = append(out, Listed{SDKID: r.SDKID, MeshName: r.MeshName})
		}
	}
	return out, nil
}

// FindID returns the id of the first listed object whose id equals id
// without regard to case, as the gateway matches ids.
func FindID(listed []Listed, id string) (Listed, bool) {
	for _, l := range listed {
		if strings.EqualFold(l.SDKID, id) {
			return l, true
		}
	}
	return Listed{}, false
}

// FindMesh returns the first listed object whose mesh name equals mesh exactly.
func FindMesh(listed []Listed, mesh string) (Listed, bool) {
	for _, l := range listed {
		if l.MeshName == mesh {
			return l, true
		}
	}
	return Listed{}, false
}
