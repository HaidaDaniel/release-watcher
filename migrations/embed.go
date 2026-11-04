package migrations

import (
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.sql
var FS embed.FS

// Migration is one append-only schema change.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// All returns the embedded migrations ordered by version.
func All() ([]Migration, error) {
	entries, err := FS.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var out []Migration
	seen := map[int]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("invalid migration version in %q", entry.Name())
		}
		if previous, ok := seen[version]; ok {
			return nil, fmt.Errorf("duplicate migration version %d: %q and %q", version, previous, entry.Name())
		}
		seen[version] = entry.Name()
		data, err := FS.ReadFile(entry.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, Migration{Version: version, Name: entry.Name(), SQL: string(data)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}
