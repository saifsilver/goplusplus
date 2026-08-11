package config

import (
	"strings"
	"testing"
)

func FuzzDotEnvLine(f *testing.F) {
	for _, seed := range []string{"KEY=value", "# comment", "BAD LINE", "SECRET='value'", ""} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, line string) {
		if len(line) > 4096 {
			return
		}
		_, _ = parseDotEnv(strings.NewReader(line))
	})
}
