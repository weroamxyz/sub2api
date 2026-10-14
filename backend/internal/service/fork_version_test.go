//go:build unit

package service

import (
	"cmp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestForkVersionCompareOrdering(t *testing.T) {
	ordered := []string{"0.2.16", "0.2.16-ace.1", "0.2.16-ace.2", "0.2.16-ace.10", "0.2.17", "0.2.17-ace.1"}

	for i, a := range ordered {
		for j, b := range ordered {
			require.Equal(t, cmp.Compare(i, j), compareVersions(a, b), "compareVersions(%q, %q)", a, b)
		}
	}
}

func TestForkVersionCompareEquivalents(t *testing.T) {
	pairs := [][2]string{
		{"v0.2.16-ace.1", "0.2.16-ace.1"},
		{"0.1.146-rc1", "0.1.146"},
		{"0.0.0-dev", "0.0.0"},
		{"0.2.16-ace.0", "0.2.16"},
	}

	for _, p := range pairs {
		require.Zero(t, compareVersions(p[0], p[1]), "compareVersions(%q, %q)", p[0], p[1])
		require.Zero(t, compareVersions(p[1], p[0]), "compareVersions(%q, %q)", p[1], p[0])
	}
}

func TestForkVersionParseSuffix(t *testing.T) {
	cases := map[string]int{
		"0.2.16-ace.3":   3,
		"v0.2.16-ace.12": 12,
		"0.2.16":         0,
		"0.1.146-rc1":    0,
		"0.0.0-dev":      0,
		"0.2.16-ace.0":   0,
		"0.2.16-ace.x":   0,
		"0.2.16-ace.+1":  0,
		"0.2.16-ace.":    0,
		"0.2.16-rc.2":    0,
	}

	for in, want := range cases {
		require.Equal(t, want, parseForkSuffix(in), "parseForkSuffix(%q)", in)
	}
}

func TestForkVersionStripSuffix(t *testing.T) {
	cases := map[string]string{
		"0.2.16-ace.1":  "0.2.16",
		"v0.2.16-ace.2": "v0.2.16",
		"0.2.16":        "0.2.16",
		"0.1.146-rc1":   "0.1.146-rc1",
		"0.2.16-ace.0":  "0.2.16-ace.0",
	}

	for in, want := range cases {
		require.Equal(t, want, stripForkVersionSuffix(in), "stripForkVersionSuffix(%q)", in)
	}
}
