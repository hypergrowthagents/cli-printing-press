package generator

import (
	"path/filepath"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/naming"
	"github.com/mvanhorn/cli-printing-press/v4/internal/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A resource whose commands all live in sub-resources has no endpoints of its
// own. The Command Reference emitted its heading and nothing else, so an agent
// reading the SKILL saw an empty group and never learned the nested commands
// exist.
func TestSkillCommandReferenceRecursesIntoSubResources(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("subresource-reference")
	apiSpec.Resources["booking-provider"] = spec.Resource{
		Description: "Booking providers",
		// No endpoints of its own — every command is nested.
		SubResources: map[string]spec.Resource{
			"bookings": {
				Description: "Provider bookings",
				Endpoints: map[string]spec.Endpoint{
					"get-list": {Method: "GET", Path: "/booking-provider/{id}/bookings", Description: "List provider bookings"},
				},
			},
		},
	}

	outputDir := filepath.Join(t.TempDir(), naming.CLI(apiSpec.Name))
	require.NoError(t, New(apiSpec, outputDir).Generate())

	skill := readGeneratedFile(t, outputDir, "SKILL.md")
	assert.Contains(t, skill, "subresource-reference-pp-cli booking-provider bookings get-list",
		"the nested command must appear in the Command Reference")
	assert.Contains(t, skill, "List provider bookings")
}

// A resource that has both its own endpoints and sub-resources must list both.
func TestSkillCommandReferenceKeepsOwnEndpointsAlongsideSubResources(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("mixed-reference")
	apiSpec.Resources["jobs"] = spec.Resource{
		Description: "Jobs",
		// Two endpoints so the resource is not promoted to a top-level
		// command, which would render as `jobs` rather than `jobs get-list`.
		Endpoints: map[string]spec.Endpoint{
			"get-list": {Method: "GET", Path: "/jobs", Description: "List jobs"},
			"get":      {Method: "GET", Path: "/jobs/{id}", Description: "Get one job"},
		},
		SubResources: map[string]spec.Resource{
			"notes": {
				Description: "Job notes",
				Endpoints: map[string]spec.Endpoint{
					"get-list": {Method: "GET", Path: "/jobs/{id}/notes", Description: "List job notes"},
				},
			},
		},
	}

	outputDir := filepath.Join(t.TempDir(), naming.CLI(apiSpec.Name))
	require.NoError(t, New(apiSpec, outputDir).Generate())

	skill := readGeneratedFile(t, outputDir, "SKILL.md")
	assert.Contains(t, skill, "mixed-reference-pp-cli jobs get-list")
	assert.Contains(t, skill, "mixed-reference-pp-cli jobs notes get-list")
}
