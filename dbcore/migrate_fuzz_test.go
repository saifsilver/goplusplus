package dbcore

import "testing"

func FuzzMigrationFilename(f *testing.F) {
	for _, seed := range []string{"0001_init.sql", "1_a.sql", "bad.sql", "0_zero.sql", "1_.sql"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, filename string) {
		if len(filename) > 256 {
			return
		}
		_, _, _ = parseMigrationFilename(filename, 0)
	})
}
