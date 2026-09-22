package generator

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/naming"
	"github.com/stretchr/testify/require"
)

// frameworkDryRunCommands are framework commands every print emits whose
// --dry-run --json output the live-dogfood matrix holds to the dry-run
// contract: a single JSON object carrying dry_run:true and a non-empty action.
// Before this, doctor, sync and workflow archive printed their ordinary
// summary (no dry_run marker), and feedback list and profile list printed a
// bare array, so the matrix failed every print on them.
var frameworkDryRunCommands = []struct {
	args   []string
	action string
}{
	{args: []string{"doctor"}, action: "doctor"},
	{args: []string{"sync"}, action: "sync"},
	{args: []string{"workflow", "archive"}, action: "workflow archive"},
	{args: []string{"feedback", "list"}, action: "feedback list"},
	{args: []string{"profile", "list"}, action: "profile list"},
}

func TestFrameworkCommandsDryRunEmitJSONEnvelope(t *testing.T) {
	t.Parallel()

	apiSpec := smallReadWriteSyncableOutputSpec("framework-dry-run")
	outputDir, binaryPath := buildGeneratedBinary(t, apiSpec)
	require.FileExists(t, filepath.Join(outputDir, "internal", "cli", "channel_workflow.go"),
		"fixture must emit workflow archive for this test to cover it")
	require.FileExists(t, filepath.Join(outputDir, "internal", "cli", "sync.go"),
		"fixture must emit sync for this test to cover it")

	for _, tc := range frameworkDryRunCommands {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			args := append(append([]string{}, tc.args...), "--dry-run", "--json")
			cmd := exec.Command(binaryPath, args...)
			cmd.Env = sandboxHomeEnv(t)
			var stdout, stderr strings.Builder
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			require.NoError(t, cmd.Run(), "stdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())

			var payload map[string]any
			require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &payload),
				"stdout must be one JSON object: %q", stdout.String())
			require.Equal(t, true, payload["dry_run"], "dry_run must be true: %v", payload)
			require.Equal(t, tc.action, payload["action"])
		})
	}

	// doctor must not claim the API is reachable when --dry-run sent nothing.
	t.Run("doctor does not probe under dry-run", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "doctor", "--dry-run", "--json")
		cmd.Env = sandboxHomeEnv(t)
		out, err := cmd.Output()
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(out, &payload))
		api, _ := payload["api"].(string)
		require.NotContains(t, api, "reachable", "a dry-run doctor contacted nothing")
		require.Contains(t, api, "dry-run")
	})
}

// TestFrameworkCommandsWithoutDryRunKeepTheirShape pins that the envelope is
// dry-run only: a normal invocation still prints the command's own output.
func TestFrameworkCommandsWithoutDryRunKeepTheirShape(t *testing.T) {
	t.Parallel()

	apiSpec := smallReadWriteSyncableOutputSpec("framework-no-dry-run")
	outputDir := filepath.Join(t.TempDir(), naming.CLI(apiSpec.Name))
	require.NoError(t, New(apiSpec, outputDir).Generate())

	for _, name := range []string{"feedback.go", "profile.go"} {
		src := readGeneratedFile(t, outputDir, "internal", "cli", name)
		require.Contains(t, src, "if dryRunOK(flags) {\n\t\t\t\treturn writeDryRun(cmd.OutOrStdout(), flags,",
			"%s list must hand its dry-run short-circuit to writeDryRun", name)
	}
	for _, name := range []string{"sync.go", "channel_workflow.go", "doctor.go"} {
		src := readGeneratedFile(t, outputDir, "internal", "cli", name)
		require.Contains(t, src, `["dry_run"] = true`, "%s must mark its dry-run envelope", name)
	}
}
