package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/naming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// extras.go tells the operator to edit it and carries no DO NOT EDIT banner,
// so regenerating over it destroyed exactly the migrations it asks for — with
// no patch record to restore them.
func TestStoreExtrasSurvivesRegeneration(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("store-extras-preserve")
	outputDir := filepath.Join(t.TempDir(), naming.CLI(apiSpec.Name))
	require.NoError(t, New(apiSpec, outputDir).Generate())

	extras := filepath.Join(outputDir, "internal", "store", "extras.go")
	original, err := os.ReadFile(extras)
	require.NoError(t, err, "the first generate must scaffold extras.go")

	// Stand in for an operator-added migration.
	edited := string(original) + "\n// operator migration: keep me across regen\n"
	require.NoError(t, os.WriteFile(extras, []byte(edited), 0o644))

	require.NoError(t, New(apiSpec, outputDir).Generate())

	after, err := os.ReadFile(extras)
	require.NoError(t, err)
	assert.Equal(t, edited, string(after),
		"a regen must not destroy hand-added migrations in extras.go")
}

// The scaffold must still appear on a clean print, or novel commands have no
// place to declare their tables.
func TestStoreExtrasIsScaffoldedOnFirstGenerate(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("store-extras-scaffold")
	outputDir := filepath.Join(t.TempDir(), naming.CLI(apiSpec.Name))
	require.NoError(t, New(apiSpec, outputDir).Generate())

	content, err := os.ReadFile(filepath.Join(outputDir, "internal", "store", "extras.go"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "package store")
}
