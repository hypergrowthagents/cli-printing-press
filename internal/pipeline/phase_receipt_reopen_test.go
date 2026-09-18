package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A shipcheck hold completes its own phase while routing to archival. Once the
// blockers are fixed the run must be able to return to the canonical path;
// without this the alternate route is a one-way door and the ledger can never
// record that the hold was superseded.
func TestEnterPhaseReopensAPhaseThatCompletedOntoANonCanonicalRoute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receipts.jsonl")
	base := PhaseReceiptOptions{Path: path, RunID: "run-1"}

	require.NoError(t, initLedger(base, "12-shipcheck"))
	complete(t, base, "12-shipcheck", "20-promote-and-archive")

	// The canonical successor is refused, and the error says what to do.
	_, _, err := EnterPhase(withPhase(base, "13-sync-param-drop-gate"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--resume")

	// Reopening the held phase itself is allowed.
	opts := withPhase(base, "12-shipcheck")
	opts.Resume = true
	receipt, appended, err := EnterPhase(opts)
	require.NoError(t, err)
	assert.True(t, appended)
	assert.Equal(t, PhaseReceiptEntered, receipt.Event)
	assert.Equal(t, "12-shipcheck", receipt.Phase)
}

// Reopening must never become a way to skip pending work: only the phase that
// just completed can be reopened, never a later one.
func TestEnterPhaseResumeCannotSkipAheadToAnotherPhase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receipts.jsonl")
	base := PhaseReceiptOptions{Path: path, RunID: "run-1"}

	require.NoError(t, initLedger(base, "12-shipcheck"))
	complete(t, base, "12-shipcheck", "20-promote-and-archive")

	for _, phase := range []string{"13-sync-param-drop-gate", "19-polish", "21-next-steps"} {
		opts := withPhase(base, phase)
		opts.Resume = true
		_, _, err := EnterPhase(opts)
		require.Error(t, err, "--resume must not unlock %s", phase)
		assert.Contains(t, err.Error(), "transition mismatch",
			"refusal must name the transition, not some incidental validation")
	}
}

// The ordinary path is untouched: entering the named successor still needs no
// flag at all.
func TestEnterPhaseStillFollowsTheNamedSuccessorWithoutResume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receipts.jsonl")
	base := PhaseReceiptOptions{Path: path, RunID: "run-1"}

	require.NoError(t, initLedger(base, "12-shipcheck"))
	complete(t, base, "12-shipcheck", "13-sync-param-drop-gate")

	receipt, appended, err := EnterPhase(withPhase(base, "13-sync-param-drop-gate"))
	require.NoError(t, err)
	assert.True(t, appended)
	assert.Equal(t, "13-sync-param-drop-gate", receipt.Phase)
}

func withPhase(base PhaseReceiptOptions, phase string) PhaseReceiptOptions {
	base.Phase = phase
	return base
}

// initLedger opens a ledger and walks the canonical phase order up to the phase
// the test cares about. Init both enters and completes the first phase, so the
// walk resumes from the second.
func initLedger(base PhaseReceiptOptions, phase string) error {
	if _, _, err := InitPhaseReceipts(withPhase(base, printingPressReceiptPhases[0])); err != nil {
		return err
	}
	for i := 1; i < len(printingPressReceiptPhases); i++ {
		current := printingPressReceiptPhases[i]
		opts := withPhase(base, current)
		if _, _, err := EnterPhase(opts); err != nil {
			return err
		}
		if current == phase {
			return nil
		}
		opts.Next = printingPressReceiptPhases[i+1]
		if _, _, err := CompletePhase(opts, false); err != nil {
			return err
		}
	}
	return nil
}

func complete(t *testing.T, base PhaseReceiptOptions, phase, next string) {
	t.Helper()
	opts := withPhase(base, phase)
	opts.Next = next
	// The ledger requires a reason whenever a handoff leaves the canonical
	// order, which is exactly the hold these tests reproduce.
	opts.Note = "hold: two legs red"
	_, _, err := CompletePhase(opts, false)
	require.NoError(t, err)
}

// The writer and the reader must agree: a ledger containing a reopen has to
// survive being read back, or the reopen corrupts the run's audit trail.
func TestReadPhaseReceiptsAcceptsALedgerContainingAReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receipts.jsonl")
	base := PhaseReceiptOptions{Path: path, RunID: "run-1"}

	require.NoError(t, initLedger(base, "12-shipcheck"))
	complete(t, base, "12-shipcheck", "20-promote-and-archive")

	opts := withPhase(base, "12-shipcheck")
	opts.Resume = true
	_, _, err := EnterPhase(opts)
	require.NoError(t, err)

	receipts, err := ReadPhaseReceipts(path)
	require.NoError(t, err, "a reopened ledger must read back cleanly")
	assert.Equal(t, PhaseReceiptEntered, receipts[len(receipts)-1].Event)

	// And the reopened phase can then complete onto its canonical successor.
	done := withPhase(base, "12-shipcheck")
	done.Next = "13-sync-param-drop-gate"
	_, _, err = CompletePhase(done, false)
	require.NoError(t, err)

	receipts, err = ReadPhaseReceipts(path)
	require.NoError(t, err)
	last := receipts[len(receipts)-1]
	assert.Equal(t, PhaseReceiptCompleted, last.Event)
	assert.Equal(t, "13-sync-param-drop-gate", last.Next)
}

// The reader must still reject a genuine skip, which is the invariant the
// original rule protected.
func TestReadPhaseReceiptsStillRejectsASkippedPhase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receipts.jsonl")
	base := PhaseReceiptOptions{Path: path, RunID: "run-1"}
	require.NoError(t, initLedger(base, "12-shipcheck"))
	complete(t, base, "12-shipcheck", "13-sync-param-drop-gate")

	// Hand-append an entry for a phase nobody handed off to. Sequence must be
	// the real next one so the forgery is rejected by the transition rule
	// rather than by an earlier structural check.
	existing, err := ReadPhaseReceipts(path)
	require.NoError(t, err)
	forged := fmt.Sprintf(`{"schema_version":1,"sequence":%d,`, existing[len(existing)-1].Sequence+1) + `"run_id":"run-1","phase":"19-polish","event":"entered","phase_file":"phases/19-polish.md","recorded_at":"2026-09-18T00:00:00Z"}`
	f, openErr := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, openErr)
	_, err = f.WriteString(forged + "\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	_, err = ReadPhaseReceipts(path)
	require.Error(t, err, "a phase nobody handed off to must still be rejected")
	assert.Contains(t, err.Error(), "does not match previous next phase")
}
