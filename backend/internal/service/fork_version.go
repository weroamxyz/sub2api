package service

import (
	"strconv"
	"strings"
)

// forkVersionSuffix marks releases of the Simooonn/sub2api fork, e.g. "0.2.16-ace.1".
// A fork release ranks above the upstream release with the same x.y.z and below the
// next upstream x.y.z. This intentionally differs from semver prerelease ordering.
const forkVersionSuffix = "ace"

// parseForkSuffix returns N for a "-ace.N" suffix (N >= 1) and 0 for anything else.
func parseForkSuffix(v string) int {
	_, suffix, ok := strings.Cut(strings.TrimSpace(v), "-")
	if !ok {
		return 0
	}
	label, num, ok := strings.Cut(suffix, ".")
	if !ok || label != forkVersionSuffix || num == "" {
		return 0
	}
	for _, r := range num {
		if r < '0' || r > '9' {
			return 0
		}
	}
	n, err := strconv.Atoi(num)
	if err != nil || n < 1 {
		return 0
	}
	return n
}

// stripForkVersionSuffix removes a valid "-ace.N" suffix and returns any other input unchanged.
func stripForkVersionSuffix(v string) string {
	if parseForkSuffix(v) == 0 {
		return v
	}
	base, _, _ := strings.Cut(strings.TrimSpace(v), "-")
	return base
}
