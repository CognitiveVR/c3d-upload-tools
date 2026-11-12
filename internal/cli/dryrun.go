package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/pflag"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/gateway"
)

// dryRunVersion stands in for a version number a dry run cannot know because
// only the gateway can say which version is the latest.
const dryRunVersion = "<resolved at run time>"

// versionText is the version a dry run shows: wanted when given, else
// dryRunVersion.
func versionText(wanted int, given bool) string {
	if given {
		return strconv.Itoa(wanted)
	}
	return dryRunVersion
}

type dryRunPart struct {
	Field    string `json:"field"`
	Filename string `json:"filename"`
	Bytes    int64  `json:"bytes"`
}

type dryRunRequest struct {
	DryRun        bool         `json:"dryRun"`
	Method        string       `json:"method"`
	Path          string       `json:"path"`
	Authorization string       `json:"authorization"`
	Parts         []dryRunPart `json:"parts"`
}

func newDryRunRequest(rt *runtime, method, path string, parts []gateway.Part) dryRunRequest {
	request := dryRunRequest{
		DryRun: true, Method: method, Path: path,
		Authorization: "APIKEY:DEVELOPER " + redactKey(rt.settings.APIKey),
	}
	for _, p := range parts {
		request.Parts = append(request.Parts, dryRunPart{Field: p.Field, Filename: p.Filename, Bytes: p.Size})
	}
	return request
}

// printDryRun writes document as JSON, or in text mode the request line,
// the authorization and one line per part followed by extraText. document
// is request itself or a struct that embeds it.
func printDryRun(rt *runtime, request dryRunRequest, document any, extraText string) error {
	out := rt.streams.Stdout
	if rt.settings.Format == formatJSON {
		return json.NewEncoder(out).Encode(document)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\nAuthorization: %s\n", request.Method, request.Path, request.Authorization)
	for _, p := range request.Parts {
		fmt.Fprintf(&b, "%s  %s  %d bytes\n", p.Field, p.Filename, p.Bytes)
	}
	b.WriteString(extraText)
	_, err := io.WriteString(out, b.String())
	return err
}

// redactKey shows at most the first four characters of a key, and none of a
// key too short to keep the rest hidden.
func redactKey(key string) string {
	switch {
	case key == "":
		return "(not set)"
	case len(key) <= 4:
		return "…"
	}
	return key[:4] + "…"
}

// versionFlagValue reads --version; given is false when the flag is absent,
// and a given number below one is a usage error.
func versionFlagValue(flags *pflag.FlagSet) (wanted int, given bool, err error) {
	wanted, _ = flags.GetInt(versionFlag)
	given = flags.Changed(versionFlag)
	if given && wanted < 1 {
		return wanted, given, usageErrorf("invalid --%s %d: want a version number of 1 or more", versionFlag, wanted)
	}
	return wanted, given, nil
}
