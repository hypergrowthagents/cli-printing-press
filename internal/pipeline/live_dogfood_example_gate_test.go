package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The live matrix's help check required an Examples section on every command,
// including the ones the generator marked as having no derivable example. A
// failed help check also skips that command's happy_path, json_fidelity and
// error_path, so one unmeetable requirement withdrew most of a wide CLI's
// surface from live testing rather than exercising it.
//
// This calls the production predicate the runner uses, so it fails if the
// predicate changes; the end-to-end leg is pinned by
// TestRunLiveDogfoodSkipsMutatingCommandsWithoutRunnableExample.
func TestLiveDogfoodHelpCheckHonoursTheNoRunnableExampleAnnotation(t *testing.T) {
	t.Parallel()

	assert.False(t, liveDogfoodHelpRequiresExamples(map[string]string{noRunnableExampleAnnotation: "true"}),
		"a command that cannot carry a runnable example must not fail the help check")
	assert.True(t, liveDogfoodHelpRequiresExamples(map[string]string{}),
		"a command that could carry an example is still held to it")
	assert.True(t, liveDogfoodHelpRequiresExamples(map[string]string{noRunnableExampleAnnotation: "false"}),
		"only an explicit true opts out")
	assert.True(t, liveDogfoodHelpRequiresExamples(nil))
}

// TestHappyPathResultForMissingExample pins the read side of the rule the
// mutating branch already follows. It exercises the production helper rather
// than restating its condition, so the test fails if the helper changes.
func TestHappyPathResultForMissingExample(t *testing.T) {
	t.Parallel()

	args := []string{"demo", "get"}

	skipped := happyPathResultForMissingExample("demo get",
		map[string]string{noRunnableExampleAnnotation: "true"}, args)
	assert.Equal(t, LiveDogfoodStatusSkip, skipped.Status,
		"a command that cannot carry a runnable example has no happy path to fail")
	assert.Equal(t, reasonMissingRunnableExample, skipped.Reason)
	assert.Equal(t, LiveDogfoodTestHappy, skipped.Kind)

	for name, annotations := range map[string]map[string]string{
		"absent":         nil,
		"empty":          {},
		"explicit false": {noRunnableExampleAnnotation: "false"},
	} {
		t.Run(name, func(t *testing.T) {
			got := happyPathResultForMissingExample("demo get", annotations, args)
			assert.Equal(t, LiveDogfoodStatusFail, got.Status,
				"only an explicit true opts out; a missing example is otherwise a real finding")
			assert.Equal(t, reasonMissingRunnableExample, got.Reason)
		})
	}
}
