package cli

import (
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/spec"
)

// A multi-spec merge must rebase OAuth env var names onto the CLI name.
// Without this, the merged CLI reads the FIRST spec's prefix (e.g.
// ACCOUNTING_CLIENT_ID for a CLI named servicetitan) while every other
// generated surface — app key, template vars, docs — uses the CLI name.
// The mismatch is silent: the CLI builds, tests pass, and authentication
// fails at runtime against variables the operator never set.
func TestMergeSpecs_RebasesAuthEnvPrefixToCLIName(t *testing.T) {
	newSpec := func(name string, envVars []string) *spec.APISpec {
		return &spec.APISpec{
			Name:    name,
			BaseURL: "https://api.example.com/" + name,
			Auth: spec.AuthConfig{
				Type:     "oauth2",
				TokenURL: "https://auth.example.com/token",
				EnvVars:  envVars,
				EnvVarSpecs: []spec.AuthEnvVar{
					{Name: envVars[0], Required: true},
					{Name: envVars[1], Required: true, Sensitive: true},
				},
			},
			Resources: map[string]spec.Resource{},
			Types:     map[string]spec.TypeDef{},
		}
	}

	specs := []*spec.APISpec{
		newSpec("Accounting", []string{"ACCOUNTING_CLIENT_ID", "ACCOUNTING_CLIENT_SECRET"}),
		newSpec("CRM", []string{"CRM_CLIENT_ID", "CRM_CLIENT_SECRET"}),
	}

	merged := mergeSpecsWithOptions(specs, "servicetitan", mergeSpecOptions{})

	wantEnvVars := []string{"SERVICETITAN_CLIENT_ID", "SERVICETITAN_CLIENT_SECRET"}
	if len(merged.Auth.EnvVars) != len(wantEnvVars) {
		t.Fatalf("EnvVars = %v, want %v", merged.Auth.EnvVars, wantEnvVars)
	}
	for i, want := range wantEnvVars {
		if merged.Auth.EnvVars[i] != want {
			t.Errorf("EnvVars[%d] = %q, want %q", i, merged.Auth.EnvVars[i], want)
		}
	}
	for i, want := range wantEnvVars {
		if merged.Auth.EnvVarSpecs[i].Name != want {
			t.Errorf("EnvVarSpecs[%d].Name = %q, want %q", i, merged.Auth.EnvVarSpecs[i].Name, want)
		}
	}
}

// The auth model may come from a spec other than specs[0] when the first
// spec carries no auth. The rebase must use that spec's name as the source
// prefix, not specs[0]'s.
func TestMergeSpecs_RebasesAuthEnvPrefixFromTheAuthBearingSpec(t *testing.T) {
	noAuth := &spec.APISpec{
		Name:      "Telecom",
		BaseURL:   "https://api.example.com/telecom",
		Resources: map[string]spec.Resource{},
		Types:     map[string]spec.TypeDef{},
	}
	withAuth := &spec.APISpec{
		Name:    "Pricebook",
		BaseURL: "https://api.example.com/pricebook",
		Auth: spec.AuthConfig{
			Type:             "oauth2",
			AuthorizationURL: "https://auth.example.com/authorize",
			TokenURL:         "https://auth.example.com/token",
			EnvVars:          []string{"PRICEBOOK_CLIENT_ID"},
			EnvVarSpecs:      []spec.AuthEnvVar{{Name: "PRICEBOOK_CLIENT_ID", Required: true}},
		},
		Resources: map[string]spec.Resource{},
		Types:     map[string]spec.TypeDef{},
	}

	merged := mergeSpecsWithOptions([]*spec.APISpec{noAuth, withAuth}, "servicetitan", mergeSpecOptions{})

	if len(merged.Auth.EnvVars) != 1 || merged.Auth.EnvVars[0] != "SERVICETITAN_CLIENT_ID" {
		t.Errorf("EnvVars = %v, want [SERVICETITAN_CLIENT_ID]", merged.Auth.EnvVars)
	}
}

// An env var that does not carry the source spec's prefix is left alone —
// rebasing must not rewrite unrelated names.
func TestMergeSpecs_LeavesUnprefixedAuthEnvVarsAlone(t *testing.T) {
	s1 := &spec.APISpec{
		Name:    "Accounting",
		BaseURL: "https://api.example.com/accounting",
		Auth: spec.AuthConfig{
			Type:     "oauth2",
			TokenURL: "https://auth.example.com/token",
			EnvVars:  []string{"ACCOUNTING_CLIENT_ID", "GLOBAL_SHARED_TOKEN"},
			EnvVarSpecs: []spec.AuthEnvVar{
				{Name: "ACCOUNTING_CLIENT_ID", Required: true},
				{Name: "GLOBAL_SHARED_TOKEN", Required: true},
			},
		},
		Resources: map[string]spec.Resource{},
		Types:     map[string]spec.TypeDef{},
	}
	s2 := &spec.APISpec{
		Name:      "CRM",
		BaseURL:   "https://api.example.com/crm",
		Resources: map[string]spec.Resource{},
		Types:     map[string]spec.TypeDef{},
	}

	merged := mergeSpecsWithOptions([]*spec.APISpec{s1, s2}, "servicetitan", mergeSpecOptions{})

	want := []string{"SERVICETITAN_CLIENT_ID", "GLOBAL_SHARED_TOKEN"}
	for i, w := range want {
		if merged.Auth.EnvVars[i] != w {
			t.Errorf("EnvVars[%d] = %q, want %q", i, merged.Auth.EnvVars[i], w)
		}
	}
}
