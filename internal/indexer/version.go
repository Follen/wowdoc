package indexer

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/follenfang/wowdoc/internal/catalog"
)

// BuildInterfaceFromVersion derives a game Interface version (for example
// "120100") from a source version.txt payload (for example "12.1.0.69814").
// The Interface number is Major*10000 + Minor*100 + Patch, which matches the
// values AddOn TOC files declare and the values validation compares.
//
// Only the source itself is authoritative: a Tag or file name is never parsed
// here, so third-party AddOn versions can never be mistaken for a game build.
// It returns "" when the payload does not start with Major.Minor.Patch
// numerics.
func BuildInterfaceFromVersion(data []byte) string {
	line := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
	line = strings.TrimPrefix(line, "\xef\xbb\xbf")
	line = strings.TrimSpace(line)
	parts := strings.Split(line, ".")
	if len(parts) < 3 {
		return ""
	}
	nums := make([]int, 0, 3)
	for _, part := range parts[:3] {
		n, ok := leadingInt(part)
		if !ok {
			return ""
		}
		nums = append(nums, n)
	}
	return strconv.Itoa(nums[0]*10000 + nums[1]*100 + nums[2])
}

func leadingInt(s string) (int, bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(s[:i])
	if err != nil {
		return 0, false
	}
	return n, true
}

// snapshotBuildInterface reads the root version.txt of the indexed source and
// derives its Interface version. It returns "" when the source legitimately
// carries no usable build version, and an error when a version.txt exists but
// cannot be read: caching that failure as "no version" would permanently deny
// the snapshot its Interface evidence.
func snapshotBuildInterface(ctx context.Context, opts BuildOptions, entries []Entry) (string, error) {
	// AddOn release versions are not game build evidence.
	if !catalog.IsGameSource(opts.SourceID) {
		return "", nil
	}
	for _, entry := range entries {
		if strings.Contains(entry.Path, "/") || !strings.EqualFold(entry.Path, "version.txt") {
			continue
		}
		data, err := opts.Input.Read(ctx, entry)
		if err != nil {
			return "", fmt.Errorf("read source version.txt: %w", err)
		}
		return BuildInterfaceFromVersion(data), nil
	}
	return "", nil
}
