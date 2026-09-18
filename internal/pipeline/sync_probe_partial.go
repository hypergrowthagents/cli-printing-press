package pipeline

import (
	"path/filepath"
	"strings"
	"time"
)

// syncProbeFallbackResource is the resource name the data-pipeline probe used
// before it could read the CLI's own declarations. It is a GitHub-ism, so on
// every other CLI the scoping attempt failed by construction and the probe
// spent an attempt learning nothing.
const syncProbeFallbackResource = "repos"

// firstDeclaredSyncResource returns a resource the generated CLI actually
// syncs, so the probe's narrow first attempt exercises the pipeline instead of
// guaranteeing a fall-through. An empty return means the declarations could not
// be read and the caller should keep the historical fallback.
func firstDeclaredSyncResource(cliDir string) string {
	if strings.TrimSpace(cliDir) == "" {
		return ""
	}
	content := readAllGoFiles(filepath.Join(cliDir, "internal", "cli"))
	match := defaultSyncResourcesBodyRe.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	for _, literal := range syncResourceLiteralRe.FindAllStringSubmatch(match[1], -1) {
		if len(literal) < 2 {
			continue
		}
		if name := strings.TrimSpace(literal[1]); name != "" {
			return name
		}
	}
	return ""
}

// syncProbeResource picks the resource name for the probe's scoped attempt.
func syncProbeResource(cliDir string) string {
	if name := firstDeclaredSyncResource(cliDir); name != "" {
		return name
	}
	return syncProbeFallbackResource
}

// syncProbeStoreExecuted reports whether sync got far enough to create its
// domain schema. A generated sync exits non-zero when any single resource
// fails, so on a wide CLI one unserved path makes the whole command report
// failure; breadth alone then looked like a crash. Schema in the store proves
// the pipeline ran, and lets the row checks downstream deliver the real
// verdict — including failing it — on evidence rather than on an exit code.
func syncProbeStoreExecuted(binary, dbPath string, env []string) bool {
	out, err := runCLIWithOutput(binary, []string{"sql", "--db", dbPath, syncProbeTableQuery}, env, 10*time.Second)
	if err != nil {
		return false
	}
	return len(parseSQLOutput(out)) > 0
}

// syncProbeTableQuery lists the domain tables a sync is expected to create,
// excluding sqlite internals, full-text shadow tables, and bookkeeping.
const syncProbeTableQuery = `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite%' AND name NOT LIKE '%_fts%' AND name != 'sync_state'`
