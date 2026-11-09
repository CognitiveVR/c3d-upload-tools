package object

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

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

// WithoutFiles returns the listed objects whose files array is empty, in
// list order.
func WithoutFiles(raw []byte) ([]Listed, error) {
	var rows []struct {
		SDKID    string            `json:"sdkId"`
		MeshName string            `json:"meshName"`
		Files    []json.RawMessage `json:"files"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("parse object list: %w", err)
	}
	var out []Listed
	for _, r := range rows {
		if len(r.Files) == 0 {
			out = append(out, Listed{SDKID: r.SDKID, MeshName: r.MeshName})
		}
	}
	return out, nil
}
