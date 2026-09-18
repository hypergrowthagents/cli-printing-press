package generator

import (
	"path/filepath"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/naming"
	"github.com/mvanhorn/cli-printing-press/v4/internal/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The generator declines to invent an example when it cannot derive a runnable
// one. Downstream, dogfood's example-coverage leg needs to tell that deliberate
// decline apart from a command that simply lost its example, so the decision is
// emitted as an annotation rather than left implicit in the absence of an
// Example field.
func TestGeneratedCommandAnnotatesWhenNoRunnableExampleIsDerivable(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("no-example-annotation")
	apiSpec.Resources["invoices"] = spec.Resource{
		Description: "Invoices",
		Endpoints: map[string]spec.Endpoint{
			// Nothing in the spec supplies an invoice id, so no runnable
			// example exists for this command.
			"get_by_id": {
				Method:      "GET",
				Path:        "/v1/invoices/{invoice_id}",
				Description: "Get one invoice",
				Params: []spec.Param{
					{Name: "invoice_id", Type: "string", Required: true, Positional: true, PathParam: true},
				},
			},
			// A bare collection read needs no input, so an example is derivable.
			"list": {
				Method:      "GET",
				Path:        "/v1/invoices",
				Description: "List invoices",
			},
		},
	}

	outputDir := filepath.Join(t.TempDir(), naming.CLI(apiSpec.Name))
	require.NoError(t, New(apiSpec, outputDir).Generate())

	byID := readGeneratedFile(t, outputDir, "internal", "cli", "invoices_get_by_id.go")
	assert.NotContains(t, byID, "Example:",
		"an id-addressed read has no derivable example to emit")
	assert.Contains(t, byID, `"pp:no-runnable-example": "true"`,
		"the decline must be visible to the verifier that scores example coverage")

	list := readGeneratedFile(t, outputDir, "internal", "cli", "invoices_list.go")
	assert.Contains(t, list, "Example:")
	assert.NotContains(t, list, "pp:no-runnable-example",
		"a command that got an example must stay in the coverage denominator")

	requireGeneratedCompiles(t, outputDir)
}
