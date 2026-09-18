package pipeline

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeSyncFixture(t *testing.T, resources []string) string {
	t.Helper()
	dir := t.TempDir()
	cliDir := filepath.Join(dir, "internal", "cli")
	if err := os.MkdirAll(cliDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "package cli\n\nfunc defaultSyncResources() []string {\n\treturn []string{\n"
	for _, r := range resources {
		body += "\t\t\"" + r + "\",\n"
	}
	body += "\t}\n}\n"
	if err := os.WriteFile(filepath.Join(cliDir, "sync.go"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSyncResourceCount(t *testing.T) {
	dir := writeSyncFixture(t, []string{"repos", "issues", "pulls"})
	if got := syncResourceCount(dir); got != 3 {
		t.Errorf("syncResourceCount = %d, want 3", got)
	}
}

func TestSyncResourceCount_UnknownDirIsZero(t *testing.T) {
	if got := syncResourceCount(t.TempDir()); got != 0 {
		t.Errorf("syncResourceCount on an empty dir = %d, want 0", got)
	}
}

// A small CLI keeps the historical 30s budget; a large one gets a budget that
// scales, because the probe walks every declared resource in sequence.
func TestSyncProbeTimeout_ScalesWithResourceCount(t *testing.T) {
	small := writeSyncFixture(t, []string{"repos", "issues"})
	if got := syncProbeTimeout(small); got != syncProbeBaseTimeout {
		t.Errorf("small CLI timeout = %v, want the %v floor", got, syncProbeBaseTimeout)
	}

	many := make([]string, 222)
	for i := range many {
		many[i] = "resource-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
	}
	large := writeSyncFixture(t, many)
	largeTimeout := syncProbeTimeout(large)
	if largeTimeout <= syncProbeBaseTimeout {
		t.Errorf("222-resource CLI timeout = %v, want more than the %v floor", largeTimeout, syncProbeBaseTimeout)
	}
	if largeTimeout > syncProbeMaxTimeout {
		t.Errorf("timeout = %v, want it capped at %v", largeTimeout, syncProbeMaxTimeout)
	}
}

func TestSyncProbeTimeout_IsCapped(t *testing.T) {
	many := make([]string, 100000)
	for i := range many {
		many[i] = "r"
	}
	dir := writeSyncFixture(t, many)
	if got := syncProbeTimeout(dir); got != syncProbeMaxTimeout {
		t.Errorf("timeout for an enormous CLI = %v, want the %v cap", got, syncProbeMaxTimeout)
	}
}

func TestSyncProbeTimeout_UnknownDirUsesFloor(t *testing.T) {
	if got := syncProbeTimeout(t.TempDir()); got != syncProbeBaseTimeout {
		t.Errorf("unknown-resource timeout = %v, want the %v floor", got, syncProbeBaseTimeout)
	}
}

var _ = time.Second
