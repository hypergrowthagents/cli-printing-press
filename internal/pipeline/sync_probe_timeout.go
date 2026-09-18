package pipeline

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// The data-pipeline probe runs `sync` across every resource the generated CLI
// declares, in sequence. A fixed budget therefore fails on breadth rather than
// on behaviour: a CLI printed from a large multi-module spec can declare
// hundreds of resources, and even instant per-resource failures do not finish
// inside the historical 30s. The probe then reported "sync crashed" for what
// was only a timeout.
const (
	syncProbeBaseTimeout    = 30 * time.Second
	syncProbePerResource    = 400 * time.Millisecond
	syncProbeMaxTimeout     = 10 * time.Minute
	syncProbeScaleThreshold = 25
)

var defaultSyncResourcesBodyRe = regexp.MustCompile(`(?s)func\s+defaultSyncResources\s*\(\s*\)\s*\[\]string\s*\{.*?\[\]string\{(.*?)\}`)

var syncResourceLiteralRe = regexp.MustCompile(`"([^"]+)"`)

// syncResourceCount reports how many resources the generated CLI syncs by
// default. Zero means the count could not be determined, which callers treat
// as "use the floor" rather than as "no resources".
func syncResourceCount(cliDir string) int {
	if strings.TrimSpace(cliDir) == "" {
		return 0
	}
	content := readAllGoFiles(filepath.Join(cliDir, "internal", "cli"))
	match := defaultSyncResourcesBodyRe.FindStringSubmatch(content)
	if len(match) < 2 {
		return 0
	}
	return len(syncResourceLiteralRe.FindAllString(match[1], -1))
}

// syncProbeTimeout scales the data-pipeline sync probe budget with the number
// of resources the CLI declares. Small CLIs keep the historical floor so their
// timings do not change; large ones get room to finish, capped so a pathological
// CLI cannot stall verify indefinitely.
func syncProbeTimeout(cliDir string) time.Duration {
	count := syncResourceCount(cliDir)
	if count <= syncProbeScaleThreshold {
		return syncProbeBaseTimeout
	}
	scaled := syncProbeBaseTimeout + time.Duration(count)*syncProbePerResource
	if scaled > syncProbeMaxTimeout {
		return syncProbeMaxTimeout
	}
	return scaled
}

// syncProbeHitDeadline reports whether the probe attempts ended by being killed
// at the context deadline rather than by the CLI exiting on its own. A killed
// process is a budget problem, not a crash, and saying so keeps the reader from
// hunting a defect in the generated sync.
func syncProbeHitDeadline(errs []error) bool {
	if len(errs) == 0 {
		return false
	}
	for _, err := range errs {
		if err == nil {
			return false
		}
		msg := err.Error()
		if !strings.Contains(msg, "signal: killed") && !strings.Contains(msg, "context deadline exceeded") {
			return false
		}
	}
	return true
}
