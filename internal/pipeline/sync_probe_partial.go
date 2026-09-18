package pipeline

import (
	"os"
	"path/filepath"
	"regexp"
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
	if err == nil {
		return len(parseSQLOutput(out)) > 0
	}
	// Not every printed CLI exposes `sql`: a wide surface hides it behind the
	// orchestration MCP pattern. Treating that absence as an empty store would
	// call every such CLI's pipeline a crash, so fall back to the store file,
	// which sync creates lazily and only once it is really running.
	info, statErr := os.Stat(dbPath)
	return statErr == nil && info.Size() > 0
}

// syncProbeAuthBlocked reports whether every sync attempt failed because the
// CLI could not authenticate. verify runs unauthenticated, so against an API
// that rejects anonymous reads every resource fails for one reason that says
// nothing about the print. That is not a crash and not a row shortfall — it is
// a pipeline nobody can verify without credentials, and saying so keeps the
// reader from hunting a defect in generated sync code that works as written.
func syncProbeAuthBlocked(errs []error) bool {
	if len(errs) == 0 {
		return false
	}
	sawAuth := false
	for _, err := range errs {
		if err == nil {
			return false
		}
		for _, status := range syncErrorStatusRe.FindAllStringSubmatch(err.Error(), -1) {
			if len(status) < 2 {
				continue
			}
			switch status[1] {
			case "401", "403":
				sawAuth = true
			default:
				// A non-auth failure is mixed in, so credentials are not the
				// whole story and the ordinary verdicts should speak.
				return false
			}
		}
	}
	return sawAuth
}

// syncErrorStatusRe pulls the HTTP status out of the sync_error events the
// generated sync emits, which arrive inside the probe's wrapped exec error.
var syncErrorStatusRe = regexp.MustCompile(`"status"\s*:\s*(\d{3})`)

// syncProbeTableQuery lists the domain tables a sync is expected to create,
// excluding sqlite internals, full-text shadow tables, and bookkeeping.
const syncProbeTableQuery = `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite%' AND name NOT LIKE '%_fts%' AND name != 'sync_state'`
