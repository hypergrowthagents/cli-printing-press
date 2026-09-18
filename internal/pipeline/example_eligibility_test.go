package pipeline

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeAgentContextStub(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test uses a shell script as the fake binary; skip on Windows")
	}
	binPath := filepath.Join(t.TempDir(), "fakebin")
	require.NoError(t, os.WriteFile(binPath, []byte("#!/bin/sh\ncat <<'EOF'\n"+body+"\nEOF\n"), 0o755))
	return binPath
}

// A command the generator could not give a runnable example must not count
// against example coverage. It is not a gap the generator can close: the
// command needs an id or a body no spec value supplies.
func TestDiscoverExampleCheckCommandsExcludesCommandsWithNoDerivableExample(t *testing.T) {
	t.Parallel()

	binPath := writeAgentContextStub(t, `{
  "commands": [
    {"name": "invoices", "subcommands": [
      {"name": "get-list"},
      {"name": "get-by-id", "annotations": {"pp:no-runnable-example": "true"}},
      {"name": "create", "annotations": {"pp:no-runnable-example": "true"}}
    ]}
  ]
}`)

	paths, ineligible, err := discoverExampleCheckCommands(binPath)

	require.NoError(t, err)
	assert.Equal(t, [][]string{{"invoices", "get-list"}}, paths)
	assert.Equal(t, 2, ineligible)
}

// The gate keeps its teeth: a command with no annotation is one the generator
// could have given an example, so it stays in the denominator.
func TestDiscoverExampleCheckCommandsKeepsUnannotatedCommands(t *testing.T) {
	t.Parallel()

	binPath := writeAgentContextStub(t, `{
  "commands": [
    {"name": "invoices", "subcommands": [{"name": "get-list"}, {"name": "get-summary"}]}
  ]
}`)

	paths, ineligible, err := discoverExampleCheckCommands(binPath)

	require.NoError(t, err)
	assert.Equal(t, [][]string{{"invoices", "get-list"}, {"invoices", "get-summary"}}, paths)
	assert.Zero(t, ineligible)
}

// Filtering has to happen before sampling. Sampling first would spend the
// ten-command budget on commands that cannot carry an example, leaving a
// sample too small to say anything about the print.
func TestDiscoverExampleCheckCommandsFiltersBeforeSampling(t *testing.T) {
	t.Parallel()

	body := `{"commands": [{"name": "invoices", "subcommands": [`
	// 30 ineligible commands sorted ahead of the 12 eligible ones, so a
	// sampler running first would return almost nothing usable.
	for i := 0; i < 30; i++ {
		body += `{"name": "aaa-` + string(rune('a'+i%26)) + string(rune('a'+i/26)) + `", "annotations": {"pp:no-runnable-example": "true"}},`
	}
	for i := 0; i < 12; i++ {
		body += `{"name": "zzz-` + string(rune('a'+i)) + `"},`
	}
	body = body[:len(body)-1] + `]}]}`

	paths, ineligible, err := discoverExampleCheckCommands(writeAgentContextStub(t, body))

	require.NoError(t, err)
	assert.Equal(t, 30, ineligible)
	assert.Len(t, paths, exampleCheckSampleSize)
	for _, path := range paths {
		assert.NotContains(t, path[len(path)-1], "aaa-",
			"an ineligible command reached the sample: %v", path)
	}
}

// The coverage rule is guarded by Tested > 0, so a CLI whose commands are all
// ineligible cannot trip it. Lock that guard: without it, excluding commands
// from the denominator would turn into a new way to fail the gate.
func TestExampleCoverageRuleCannotFireWhenNothingIsEligible(t *testing.T) {
	t.Parallel()

	coverageRuleFires := func(r ExampleCheckResult) bool {
		return r.Tested > 0 && (r.WithExamples*100/r.Tested) < 50
	}

	assert.False(t, coverageRuleFires(ExampleCheckResult{Tested: 0, Ineligible: 7}))
	assert.False(t, coverageRuleFires(ExampleCheckResult{Tested: 10, WithExamples: 9, Ineligible: 400}),
		"exclusions must not rescue a print whose eligible commands lack examples")
	assert.True(t, coverageRuleFires(ExampleCheckResult{Tested: 10, WithExamples: 4, Ineligible: 400}),
		"an eligible command with no example is still a real gap")
}
