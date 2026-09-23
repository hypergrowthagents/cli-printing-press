package generator

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/naming"
	"github.com/mvanhorn/cli-printing-press/v4/internal/spec"
	"github.com/stretchr/testify/require"
)

// Hand customizations used to edit generated files at a call site, which every
// regeneration wiped. The generated client and config now expose hooks, so a
// hand-owned file can attach a request gate, a config check and a token sink
// from init() and survive regeneration untouched.
func TestExtensionHooksLetHandOwnedFilesCustomizeWithoutEditingGeneratedCode(t *testing.T) {
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1"}`))
	}))
	t.Cleanup(server.Close)

	apiSpec := minimalSpec("hooks-api")
	apiSpec.BaseURL = server.URL
	apiSpec.Learn.Disabled = true
	apiSpec.Resources = map[string]spec.Resource{
		"items": {
			Description: "Items",
			Endpoints: map[string]spec.Endpoint{
				"list":   {Method: http.MethodGet, Path: "/items", Description: "List items"},
				"create": {Method: http.MethodPost, Path: "/items", Description: "Create an item", Body: []spec.Param{{Name: "name", Type: "string", Description: "Name"}}},
			},
		},
	}

	outputDir := filepath.Join(t.TempDir(), naming.CLI(apiSpec.Name))
	require.NoError(t, New(apiSpec, outputDir).Generate())

	clientSrc := readGeneratedFile(t, outputDir, "internal", "client", "client.go")
	require.Contains(t, clientSrc, "func RegisterRequestGate(gate RequestGate)")
	require.Contains(t, clientSrc, "func SetMintedTokenSink(")
	require.Contains(t, readGeneratedFile(t, outputDir, "internal", "config", "config.go"), "func RegisterLoadHook(hook func(*Config) error)")

	module := naming.CLI(apiSpec.Name)
	writeHandFile(t, outputDir, "internal/client/hand_gate.go", `package client

import (
	"errors"
	"os"
)

func init() {
	RegisterRequestGate(func(info RequestInfo) error {
		if info.Method == "POST" && os.Getenv("HOOKS_API_BLOCK") == "1" {
			return errors.New("hand gate refused POST")
		}
		return nil
	})
}
`)
	writeHandFile(t, outputDir, "internal/config/hand_check.go", `package config

import (
	"errors"
	"os"
)

func init() {
	RegisterLoadHook(func(cfg *Config) error {
		if os.Getenv("HOOKS_API_BAD_CONFIG") == "1" {
			return errors.New("hand load hook refused config")
		}
		return nil
	})
}
`)
	_ = module
	requireGeneratedCompiles(t, outputDir)

	binaryPath := filepath.Join(outputDir, module)
	runGoCommand(t, outputDir, "build", "-o", binaryPath, "./cmd/"+module)
	baseEnv := append(os.Environ(), "HOME="+t.TempDir(), "MYAPI_TOKEN=test-token",
		strings.ToUpper(strings.ReplaceAll(apiSpec.Name, "-", "_"))+"_BASE_URL="+server.URL)

	out, err := runGeneratedCLI(t, binaryPath, append(baseEnv, "HOOKS_API_BLOCK=1"), "items", "create", "--name", "x", "--json")
	require.Error(t, err, out)
	require.Contains(t, out, "hand gate refused POST")
	require.Equal(t, int32(0), posts.Load(), "a gated request must not reach the server")

	out, err = runGeneratedCLI(t, binaryPath, append(baseEnv, "HOOKS_API_BLOCK=1"), "items", "create", "--name", "x", "--dry-run", "--json")
	require.NoError(t, err, out)
	require.Equal(t, int32(0), posts.Load(), "--dry-run sends nothing and is not gated")

	out, err = runGeneratedCLI(t, binaryPath, baseEnv, "items", "create", "--name", "x", "--json")
	require.NoError(t, err, out)
	require.Equal(t, int32(1), posts.Load())

	out, err = runGeneratedCLI(t, binaryPath, append(baseEnv, "HOOKS_API_BAD_CONFIG=1"), "items", "list", "--json")
	require.Error(t, err, out)
	require.Contains(t, out, "hand load hook refused config")
}

func writeHandFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestMintedTokenSinkReplacesTokenPersistence(t *testing.T) {
	var sawBearer atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			_, _ = w.Write([]byte(`{"access_token":"minted-abc","expires_in":900}`))
			return
		}
		if r.Header.Get("Authorization") == "Bearer minted-abc" {
			sawBearer.Store(true)
		}
		_, _ = w.Write([]byte(`[{"id":"1"}]`))
	}))
	t.Cleanup(server.Close)

	apiSpec := minimalSpec("sink-api")
	apiSpec.BaseURL = server.URL
	apiSpec.Learn.Disabled = true
	apiSpec.Auth = spec.AuthConfig{
		Type:        "bearer_token",
		OAuth2Grant: spec.OAuth2GrantClientCredentials,
		TokenURL:    server.URL + "/token",
		EnvVarSpecs: []spec.AuthEnvVar{
			{Name: "SINK_API_CLIENT_ID", Kind: spec.AuthEnvVarKindAuthFlowInput},
			{Name: "SINK_API_CLIENT_SECRET", Kind: spec.AuthEnvVarKindAuthFlowInput, Sensitive: true},
		},
	}

	outputDir := filepath.Join(t.TempDir(), naming.CLI(apiSpec.Name))
	require.NoError(t, New(apiSpec, outputDir).Generate())
	writeHandFile(t, outputDir, "internal/client/hand_token_sink.go", `package client

import (
	"time"

	"sink-api-pp-cli/internal/config"
)

func init() {
	SetMintedTokenSink(func(cfg *config.Config, accessToken string, expiry time.Time) {
		cfg.AccessToken = accessToken
		cfg.TokenExpiry = expiry
	})
}
`)
	requireGeneratedCompiles(t, outputDir)
	module := naming.CLI(apiSpec.Name)
	binaryPath := filepath.Join(outputDir, module)
	runGoCommand(t, outputDir, "build", "-o", binaryPath, "./cmd/"+module)

	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home, "XDG_DATA_HOME="+filepath.Join(home, "data"), "XDG_CONFIG_HOME="+filepath.Join(home, "config"),
		"SINK_API_CLIENT_ID=id", "SINK_API_CLIENT_SECRET=secret", "SINK_API_BASE_URL="+server.URL)
	out, err := runGeneratedCLI(t, binaryPath, env, "items", "--json")
	require.NoError(t, err, out)
	require.True(t, sawBearer.Load(), "the minted token must still authenticate the request")

	var persisted []string
	_ = filepath.Walk(home, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if b, rerr := os.ReadFile(path); rerr == nil && strings.Contains(string(b), "minted-abc") {
				persisted = append(persisted, path)
			}
		}
		return nil
	})
	require.Empty(t, persisted, "the sink must replace on-disk token persistence")
}

// The README's manual MCP config named only the canonical credential and
// spelled endpoint template vars as <NAME>_<VAR>, while the binary reads the
// token-flow inputs, sibling headers and <NAME>_<VAR>_ID-style names; a host
// config copied from it could not authenticate.
func TestReadmeMCPConfigNamesEveryCredentialTheBinaryReads(t *testing.T) {
	t.Parallel()

	apiSpec := minimalSpec("readme-mcp")
	apiSpec.Auth = spec.AuthConfig{
		Type:        "bearer_token",
		OAuth2Grant: spec.OAuth2GrantClientCredentials,
		TokenURL:    "https://auth.example.com/connect/token",
		EnvVarSpecs: []spec.AuthEnvVar{
			{Name: "README_MCP_CLIENT_ID", Kind: spec.AuthEnvVarKindAuthFlowInput},
			{Name: "README_MCP_CLIENT_SECRET", Kind: spec.AuthEnvVarKindAuthFlowInput, Sensitive: true},
		},
		AdditionalHeaders: []spec.AdditionalAuthHeader{{
			Header: "X-App-Key", In: "header", Scheme: "apiKeyHeader",
			EnvVar: spec.AuthEnvVar{Name: "README_MCP_APP_KEY", Kind: spec.AuthEnvVarKindPerCall, Required: true, Sensitive: true},
		}},
	}

	outputDir := filepath.Join(t.TempDir(), naming.CLI(apiSpec.Name))
	gen := New(apiSpec, outputDir)
	gen.VisionSet = VisionTemplateSet{MCP: true}
	require.NoError(t, gen.Generate())

	readme := readGeneratedFile(t, outputDir, "README.md")
	for _, name := range []string{"README_MCP_CLIENT_ID", "README_MCP_CLIENT_SECRET", "README_MCP_APP_KEY"} {
		require.Contains(t, readme, `"`+name+`": "<`+strings.ToLower(name)+`>"`)
	}
	require.Contains(t, readme, "3. Fill in `README_MCP_CLIENT_ID`, `README_MCP_CLIENT_SECRET` and `README_MCP_APP_KEY` when Claude Desktop prompts you.")
}
