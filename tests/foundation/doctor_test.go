package foundation

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/HaidaDaniel/release-watcher/internal/cli"
)

func doctorCheckStatus(t *testing.T, stateDir string) map[string]string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	env := func(key string) string {
		if key == "RELEASE_WATCHER_STATE_DIR" {
			return stateDir
		}
		return ""
	}
	code := cli.Run(context.Background(), []string{"doctor", "--json"}, &stdout, &stderr, env)
	if code != 0 {
		t.Fatalf("doctor exit %d: %s", code, stderr.String())
	}
	var got struct {
		Data struct {
			Checks []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"checks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	out := map[string]string{}
	for _, c := range got.Data.Checks {
		out[c.ID] = c.Status
	}
	return out
}

func TestDoctorPathsAreIndependentOfWorkingDirectory(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	state := t.TempDir()
	rootStatus := doctorCheckStatus(t, state)
	docs := filepath.Join(repoRoot(t), "docs")
	if err := os.Chdir(docs); err != nil {
		t.Fatal(err)
	}
	subStatus := doctorCheckStatus(t, state)

	for _, id := range []string{"hooks", "ci"} {
		if rootStatus[id] != subStatus[id] {
			t.Fatalf("%s status differs by cwd: root=%q subdir=%q", id, rootStatus[id], subStatus[id])
		}
	}
}
