package model

import "fmt"

const (
	fileWarningBytes  int64 = 500 * 1000 * 1000
	totalWarningBytes int64 = 2 << 30
	totalLimitBytes   int64 = 3 << 30
)

// SizedFile is a file to upload and its size.
type SizedFile struct {
	Name  string
	Bytes int64
}

// CheckSizes warns about files of 500 MB or more and uploads of 2 GiB or
// more, and refuses an upload of totalBytes over the gateway's 3 GiB limit.
func CheckSizes(files []SizedFile, totalBytes int64) (warnings []string, err error) {
	for _, f := range files {
		if f.Bytes >= fileWarningBytes {
			warnings = append(warnings, fmt.Sprintf("%s is %d bytes, 500 MB or more; it will take a long time to upload and convert", f.Name, f.Bytes))
		}
	}
	switch {
	case totalBytes > totalLimitBytes:
		return warnings, fmt.Errorf("the upload is %d bytes, over the gateway's limit of 3 GiB (%d bytes) per request", totalBytes, totalLimitBytes)
	case totalBytes >= totalWarningBytes:
		warnings = append(warnings, fmt.Sprintf("the upload is %d bytes, 2 GiB or more; it will take a long time to upload and convert", totalBytes))
	}
	return warnings, nil
}
