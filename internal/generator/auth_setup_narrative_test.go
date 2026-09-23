package generator

import (
	"path/filepath"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/naming"
	"github.com/mvanhorn/cli-printing-press/v4/internal/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The client-credentials auth template emits no `setup` subcommand, but the
// SKILL narrative told every reader to run one. An agent following the SKILL
// hit exit 2 and landed on the auth help, whose only working paths persist a
// secret to disk.
func TestSkillOmitsAuthSetupWhenTheAuthCommandHasNone(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("cc-auth-narrative")
	apiSpec.Auth = spec.AuthConfig{
		Type:        "bearer_token",
		OAuth2Grant: spec.OAuth2GrantClientCredentials,
		TokenURL:    "https://auth.example.com/connect/token",
		EnvVars:     []string{"CC_AUTH_NARRATIVE_CLIENT_ID", "CC_AUTH_NARRATIVE_CLIENT_SECRET"},
	}

	outputDir := filepath.Join(t.TempDir(), naming.CLI(apiSpec.Name))
	require.NoError(t, New(apiSpec, outputDir).Generate())

	skill := readGeneratedFile(t, outputDir, "SKILL.md")
	assert.NotContains(t, skill, "auth setup",
		"the emitted auth command has no setup subcommand")
	assert.NotContains(t, skill, "--launch",
		"--launch belongs to auth setup and does not exist either")

	// The emitted auth command must actually lack it, or the assertion above
	// is proving nothing.
	authSrc := readGeneratedFile(t, outputDir, "internal", "cli", "auth.go")
	assert.NotContains(t, authSrc, "newAuthSetupCmd",
		"precondition: this flow's auth command emits no setup subcommand")
}

// A flow whose auth command does emit setup must keep the instruction.
func TestSkillKeepsAuthSetupWhenTheAuthCommandHasOne(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("authcode-narrative")
	apiSpec.Auth = spec.AuthConfig{
		Type:             "bearer_token",
		AuthorizationURL: "https://auth.example.com/authorize",
		TokenURL:         "https://auth.example.com/token",
		EnvVars:          []string{"AUTHCODE_NARRATIVE_TOKEN"},
	}

	outputDir := filepath.Join(t.TempDir(), naming.CLI(apiSpec.Name))
	require.NoError(t, New(apiSpec, outputDir).Generate())

	authSrc := readGeneratedFile(t, outputDir, "internal", "cli", "auth.go")
	require.Contains(t, authSrc, "newAuthSetupCmd",
		"precondition: this flow's auth command emits setup")
	assert.Contains(t, readGeneratedFile(t, outputDir, "SKILL.md"), "auth setup")
}

// authHasSetupCommand must agree with the template actually selected, since
// the two drifting apart is the defect these tests exist to prevent.
func TestAuthHasSetupCommandTracksTheSelectedTemplate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		auth     spec.AuthConfig
		template string
		hasSetup bool
	}{
		{
			name:     "client credentials",
			auth:     spec.AuthConfig{OAuth2Grant: spec.OAuth2GrantClientCredentials, TokenURL: "https://t.example.com/token"},
			template: "auth_client_credentials.go.tmpl",
			hasSetup: false,
		},
		{
			name:     "browser cookie",
			auth:     spec.AuthConfig{Type: "cookie"},
			template: "auth_browser.go.tmpl",
			hasSetup: false,
		},
		{
			name:     "authorization code",
			auth:     spec.AuthConfig{AuthorizationURL: "https://a.example.com/authorize"},
			template: "auth.go.tmpl",
			hasSetup: true,
		},
		{
			name:     "simple catch-all",
			auth:     spec.AuthConfig{Type: "api_key"},
			template: "auth_simple.go.tmpl",
			hasSetup: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			apiSpec := minimalSpec("auth-template-selection")
			apiSpec.Auth = tc.auth
			g := New(apiSpec, t.TempDir())

			assert.Equal(t, tc.template, g.authTemplateName())
			assert.Equal(t, tc.hasSetup, g.authHasSetupCommand())
		})
	}
}

// Naming only the canonical env var told a reader to set one value while the
// CLI still refused for want of the others.
func TestSkillNamesEveryRequiredAuthEnvVar(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("multi-env-auth")
	apiSpec.Auth = spec.AuthConfig{
		Type:        "bearer_token",
		OAuth2Grant: spec.OAuth2GrantClientCredentials,
		TokenURL:    "https://auth.example.com/connect/token",
		EnvVarSpecs: []spec.AuthEnvVar{
			{Name: "MULTI_ENV_AUTH_CLIENT_ID", Kind: spec.AuthEnvVarKindAuthFlowInput, Required: true},
			{Name: "MULTI_ENV_AUTH_CLIENT_SECRET", Kind: spec.AuthEnvVarKindAuthFlowInput, Required: true, Sensitive: true},
			{Name: "MULTI_ENV_AUTH_APP_KEY", Kind: spec.AuthEnvVarKindPerCall, Required: true, Sensitive: true},
			{Name: "MULTI_ENV_AUTH_OPTIONAL_HINT", Kind: spec.AuthEnvVarKindPerCall},
		},
	}

	outputDir := filepath.Join(t.TempDir(), naming.CLI(apiSpec.Name))
	require.NoError(t, New(apiSpec, outputDir).Generate())

	skill := readGeneratedFile(t, outputDir, "SKILL.md")
	for _, name := range []string{
		"MULTI_ENV_AUTH_CLIENT_ID",
		"MULTI_ENV_AUTH_CLIENT_SECRET",
		"MULTI_ENV_AUTH_APP_KEY",
	} {
		assert.Contains(t, skill, name, "every required auth env var must be named")
	}
}

// A sibling apiKey scheme carries its credential in AdditionalHeaders rather
// than the auth model's own env vars. Omitting those named two of the three
// values a reader had to set, so the CLI still refused.
func TestSkillNamesAdditionalHeaderAuthEnvVars(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("sibling-header-auth")
	apiSpec.Auth = spec.AuthConfig{
		Type:        "bearer_token",
		OAuth2Grant: spec.OAuth2GrantClientCredentials,
		TokenURL:    "https://auth.example.com/connect/token",
		EnvVarSpecs: []spec.AuthEnvVar{
			{Name: "SIBLING_HEADER_AUTH_CLIENT_ID", Kind: spec.AuthEnvVarKindAuthFlowInput, Required: true},
			{Name: "SIBLING_HEADER_AUTH_CLIENT_SECRET", Kind: spec.AuthEnvVarKindAuthFlowInput, Required: true, Sensitive: true},
		},
		AdditionalHeaders: []spec.AdditionalAuthHeader{{
			Header: "X-App-Key",
			In:     "header",
			Scheme: "apiKeyHeader",
			EnvVar: spec.AuthEnvVar{Name: "SIBLING_HEADER_AUTH_APP_KEY", Kind: spec.AuthEnvVarKindPerCall, Required: true, Sensitive: true},
		}},
	}

	outputDir := filepath.Join(t.TempDir(), naming.CLI(apiSpec.Name))
	require.NoError(t, New(apiSpec, outputDir).Generate())

	skill := readGeneratedFile(t, outputDir, "SKILL.md")
	for _, name := range []string{
		"SIBLING_HEADER_AUTH_CLIENT_ID",
		"SIBLING_HEADER_AUTH_CLIENT_SECRET",
		"SIBLING_HEADER_AUTH_APP_KEY",
	} {
		assert.Contains(t, skill, name)
	}
	// No dangling "Or set" when nothing preceded the env var block.
	assert.NotContains(t, skill, "Or set these environment variables")
}

// An agent that sets only the auth vars still 404s on every path when an
// endpoint template variable is unset, so the SKILL must name those too.
func TestSkillDocumentsEndpointTemplateVars(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("endpoint-var-skill")
	apiSpec.EndpointTemplateVars = []string{"tenant"}
	apiSpec.EndpointTemplateEnvOverrides = map[string]string{"tenant": "ENDPOINT_VAR_SKILL_TENANT_ID"}

	outputDir := filepath.Join(t.TempDir(), naming.CLI(apiSpec.Name))
	require.NoError(t, New(apiSpec, outputDir).Generate())

	skill := readGeneratedFile(t, outputDir, "SKILL.md")
	assert.Contains(t, skill, "ENDPOINT_VAR_SKILL_TENANT_ID",
		"the SKILL must name the override the generated config actually reads")
	assert.NotContains(t, skill, "ENDPOINT_VAR_SKILL_TENANT=",
		"the conventional name is not what the CLI reads")
}

// The README env table documented a conventional name nothing consults when
// the spec declared an override.
func TestReadmeEnvTableUsesTheResolvedEndpointEnvName(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("endpoint-var-readme")
	apiSpec.EndpointTemplateVars = []string{"tenant"}
	apiSpec.EndpointTemplateEnvOverrides = map[string]string{"tenant": "ENDPOINT_VAR_README_TENANT_ID"}
	apiSpec.Auth = spec.AuthConfig{
		Type:        "bearer_token",
		OAuth2Grant: spec.OAuth2GrantClientCredentials,
		TokenURL:    "https://auth.example.com/connect/token",
		EnvVarSpecs: []spec.AuthEnvVar{
			{Name: "ENDPOINT_VAR_README_CLIENT_ID", Kind: spec.AuthEnvVarKindAuthFlowInput, Required: true},
		},
		AdditionalHeaders: []spec.AdditionalAuthHeader{{
			Header: "X-App-Key",
			EnvVar: spec.AuthEnvVar{Name: "ENDPOINT_VAR_README_APP_KEY", Kind: spec.AuthEnvVarKindPerCall, Required: true, Sensitive: true},
		}},
	}

	outputDir := filepath.Join(t.TempDir(), naming.CLI(apiSpec.Name))
	require.NoError(t, New(apiSpec, outputDir).Generate())

	readme := readGeneratedFile(t, outputDir, "README.md")
	assert.Contains(t, readme, "`ENDPOINT_VAR_README_TENANT_ID`")
	assert.NotContains(t, readme, "`ENDPOINT_VAR_README_TENANT`  ")
	assert.Contains(t, readme, "`ENDPOINT_VAR_README_APP_KEY`",
		"a sibling apiKey scheme's required env var belongs in the table")
}

// The same drift survived in the runtime hints: doctor, the auth-error
// remediation and the MCP error text still told client-credentials users to
// run the missing `auth setup`. They must name the token-flow inputs instead.
func TestRuntimeAuthHintsOmitAuthSetupForClientCredentials(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("cc-auth-hint")
	apiSpec.Auth = spec.AuthConfig{
		Type:        "bearer_token",
		OAuth2Grant: spec.OAuth2GrantClientCredentials,
		TokenURL:    "https://auth.example.com/connect/token",
		EnvVarSpecs: []spec.AuthEnvVar{
			{Name: "CC_AUTH_HINT_CLIENT_ID", Kind: spec.AuthEnvVarKindAuthFlowInput, Required: false, Sensitive: false},
			{Name: "CC_AUTH_HINT_CLIENT_SECRET", Kind: spec.AuthEnvVarKindAuthFlowInput, Required: false, Sensitive: true},
		},
	}

	outputDir := filepath.Join(t.TempDir(), naming.CLI(apiSpec.Name))
	gen := New(apiSpec, outputDir)
	gen.VisionSet = VisionTemplateSet{MCP: true}
	require.NoError(t, gen.Generate())
	requireGeneratedCompiles(t, outputDir)

	want := "Set CC_AUTH_HINT_CLIENT_ID and CC_AUTH_HINT_CLIENT_SECRET; the CLI mints its own access token."
	for _, rel := range [][]string{
		{"internal", "cli", "doctor.go"},
		{"internal", "cli", "helpers.go"},
		{"internal", "mcp", "tools.go"},
	} {
		src := readGeneratedFile(t, outputDir, rel...)
		assert.NotContains(t, src, "auth setup' for credential setup steps", filepath.Join(rel...))
		assert.Contains(t, src, want, filepath.Join(rel...))
	}
}
