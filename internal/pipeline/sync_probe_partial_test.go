package pipeline

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirstDeclaredSyncResourceReadsTheCLIsOwnDeclarations(t *testing.T) {
	dir := t.TempDir()
	cliDir := filepath.Join(dir, "internal", "cli")
	require.NoError(t, os.MkdirAll(cliDir, 0o755))
	writeTestFile(t, filepath.Join(cliDir, "sync.go"), `package cli

func defaultSyncResources() []string {
	return []string{"invoices", "customers", "jobs"}
}
`)

	assert.Equal(t, "invoices", firstDeclaredSyncResource(dir))
}

func TestSyncProbeResourceFallsBackWhenDeclarationsAreUnreadable(t *testing.T) {
	assert.Equal(t, syncProbeFallbackResource, syncProbeResource(""))
	assert.Equal(t, syncProbeFallbackResource, syncProbeResource(t.TempDir()))
}

func TestSyncProbeResourcePrefersADeclaredResourceOverTheGitHubFallback(t *testing.T) {
	dir := t.TempDir()
	cliDir := filepath.Join(dir, "internal", "cli")
	require.NoError(t, os.MkdirAll(cliDir, 0o755))
	writeTestFile(t, filepath.Join(cliDir, "sync.go"), `package cli

func defaultSyncResources() []string {
	return []string{"business-units"}
}
`)

	got := syncProbeResource(dir)

	assert.Equal(t, "business-units", got)
	assert.NotEqual(t, syncProbeFallbackResource, got,
		"the probe must not spend its scoped attempt on a resource the CLI never declares")
}

// A wide CLI exits non-zero when any single resource fails. The probe used to
// read that as a crash of the whole data pipeline, which made breadth look like
// brokenness on every large multi-module print.
func TestRunDataPipelineTestReportsPartialResourceFailureRatherThanACrash(t *testing.T) {
	binary := buildPartialFailureSyncProbeBinary(t, 2)

	pass, detail := runDataPipelineTest(binary, "", "mock", os.Environ, 2)

	assert.True(t, pass, "a populated store proves the pipeline ran: %s", detail)
	assert.NotContains(t, detail, "sync crashed")
	assert.Contains(t, detail, "sync reported partial resource failure")
	assert.Contains(t, detail, "items has 2 rows")
}

// The escape hatch must not swallow a real crash: when sync exits non-zero and
// leaves no schema behind, nothing ran and the verdict stays FAIL.
func TestRunDataPipelineTestStillFailsWhenSyncLeavesNoSchema(t *testing.T) {
	binary := buildCrashingSyncProbeBinary(t)

	pass, detail := runDataPipelineTest(binary, "", "mock", os.Environ, 2)

	assert.False(t, pass)
	assert.Equal(t, "FAIL: sync crashed", detail)
}

// A partial sync that wrote schema but no rows is still a failure — the store
// check decides only whether the pipeline executed, not whether it worked.
func TestRunDataPipelineTestFailsPartialSyncThatWroteNoRows(t *testing.T) {
	binary := buildPartialFailureSyncProbeBinary(t, 0)

	pass, detail := runDataPipelineTest(binary, "", "mock", os.Environ, 2)

	assert.False(t, pass)
	assert.NotContains(t, detail, "sync crashed")
	assert.Contains(t, detail, "0 rows")
	assert.Contains(t, detail, "sync reported partial resource failure")
}

// buildPartialFailureSyncProbeBinary emits a CLI whose sync always exits 1 —
// as a generated sync does when one resource of many fails — while still
// writing its schema and itemRows rows.
func buildPartialFailureSyncProbeBinary(t *testing.T, itemRows int) string {
	t.Helper()
	return buildSyncProbeBinary(t, `
	case "sync":
		dbPath := dbArg(args[1:])
		if dbPath == "" {
			os.Exit(1)
		}
		if err := os.WriteFile(dbPath+".marker", []byte(dbPath), 0o644); err != nil {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "Error: 1 resource(s) failed to sync: widgets")
		os.Exit(1)
`, itemRows)
}

// buildCrashingSyncProbeBinary emits a CLI whose sync fails before writing
// anything, so the store stays empty.
func buildCrashingSyncProbeBinary(t *testing.T) string {
	t.Helper()
	return buildSyncProbeBinary(t, `
	case "sync":
		fmt.Fprintln(os.Stderr, "panic: nil map")
		os.Exit(2)
`, 0)
}

func buildSyncProbeBinary(t *testing.T, syncCase string, itemRows int) string {
	t.Helper()

	dir := t.TempDir()
	mainFile := filepath.Join(dir, "main.go")
	writeTestFile(t, mainFile, fmt.Sprintf(`package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		os.Exit(1)
	}
	switch args[0] {%s
	case "sql":
		dbPath := dbArg(args[1:])
		if dbPath == "" {
			os.Exit(1)
		}
		usedDB, err := os.ReadFile(dbPath + ".marker")
		if err != nil || string(usedDB) != dbPath {
			os.Exit(1)
		}
		query := args[len(args)-1]
		if strings.Contains(query, "sqlite_master") {
			fmt.Println("items")
			return
		}
		if strings.Contains(query, "count(*)") {
			fmt.Println(%d)
			return
		}
		os.Exit(1)
	case "health":
		return
	}
	os.Exit(1)
}

func dbArg(args []string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--db" {
			return args[i+1]
		}
	}
	return ""
}
`, syncCase, itemRows))
	binaryPath := filepath.Join(dir, "test-cli")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, mainFile)
	out, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "building test binary: %s", string(out))
	return binaryPath
}

// verify runs unauthenticated. Against an API that rejects anonymous reads
// every resource fails for one reason that says nothing about the print, so the
// pipeline is unverified rather than crashed.
func TestSyncProbeAuthBlockedRecognizesAnAllAuthFailureWall(t *testing.T) {
	authErr := errors.New(`exit status 1: {"event":"sync_error","resource":"bills","status":401,"title":"Application key not present"}`)

	assert.True(t, syncProbeAuthBlocked([]error{authErr}))
	assert.True(t, syncProbeAuthBlocked([]error{authErr, authErr}))
}

// A mixed failure is not an auth story, so the ordinary verdicts must speak.
func TestSyncProbeAuthBlockedIgnoresMixedFailures(t *testing.T) {
	authErr := errors.New(`exit status 1: {"event":"sync_error","status":401}`)
	serverErr := errors.New(`exit status 1: {"event":"sync_error","status":500}`)

	assert.False(t, syncProbeAuthBlocked([]error{authErr, serverErr}))
	assert.False(t, syncProbeAuthBlocked([]error{serverErr}))
	assert.False(t, syncProbeAuthBlocked(nil))
	assert.False(t, syncProbeAuthBlocked([]error{errors.New("exit status 2: panic: nil map")}),
		"a crash with no HTTP status is not an auth block")
}

// Not every printed CLI exposes `sql`. Treating that absence as an empty store
// would call every such CLI's pipeline a crash.
func TestSyncProbeStoreExecutedFallsBackToTheStoreFileWhenSQLIsAbsent(t *testing.T) {
	binary := buildCrashingSyncProbeBinary(t) // its sql probe exits non-zero
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	assert.False(t, syncProbeStoreExecuted(binary, dbPath, os.Environ()),
		"no store file means sync never ran")

	require.NoError(t, os.WriteFile(dbPath, []byte("SQLite format 3\x00"), 0o644))
	assert.True(t, syncProbeStoreExecuted(binary, dbPath, os.Environ()),
		"a written store proves sync ran even when sql cannot be queried")
}
