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
func TestLiveDogfoodHelpCheckHonoursTheNoRunnableExampleAnnotation(t *testing.T) {
	t.Parallel()

	requiresExamples := func(annotations map[string]string) bool {
		return annotations[noRunnableExampleAnnotation] != "true"
	}

	assert.False(t, requiresExamples(map[string]string{noRunnableExampleAnnotation: "true"}),
		"a command that cannot carry a runnable example must not fail the help check")
	assert.True(t, requiresExamples(map[string]string{}),
		"a command that could carry an example is still held to it")
	assert.True(t, requiresExamples(map[string]string{noRunnableExampleAnnotation: "false"}),
		"only an explicit true opts out")
	assert.True(t, requiresExamples(nil))
}
