// Package pluginloader builds a provisionhook.Registry populated with
// the gh-identity Mutator so the platform's cmd/server can wire this
// plugin into the workspace provision chain.
//
// Operators integrating the plugin:
//
//	reg, err := pluginloader.BuildRegistry()
//	if err != nil { log.Fatalf("gh-identity: %v", err) }
//	wh.SetEnvMutators(append(existingMutators, reg.Mutators()...))
//
// The plugin is INTENTIONALLY non-fatal on missing config: absent the
// optional MOLECULE_GH_IDENTITY_CONFIG_FILE env var, this still
// registers a Mutator that reads workspace-supplied roles and emits
// wrapper env — just without @me owner rewriting. Operators who want
// owner rewriting set MOLECULE_GH_IDENTITY_CONFIG_FILE.
package pluginloader

import (
	"fmt"
	"os"

	"github.com/Molecule-AI/molecule-ai-plugin-gh-identity/internal/ghidentity"
)

// Result bundles what BuildRegistry returns — a single mutator plus
// whatever config it loaded, so test harnesses can inspect both.
type Result struct {
	Mutator *ghidentity.Mutator
	Config  *ghidentity.Config
}

// BuildRegistry reads MOLECULE_GH_IDENTITY_CONFIG_FILE (optional),
// constructs the Mutator, and returns it.
//
// Error modes:
//   - config file set but unreadable → error (operator bug; fail loud)
//   - config file unset → fine, use empty map
//   - config file set but non-existent → fine, use empty map (lets you
//     point at a file that CI hasn't created yet without blocking boot)
func BuildRegistry() (*Result, error) {
	cfgPath := os.Getenv("MOLECULE_GH_IDENTITY_CONFIG_FILE")
	cfg, err := ghidentity.LoadConfig(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("gh-identity: %w", err)
	}
	return &Result{
		Mutator: &ghidentity.Mutator{Config: cfg},
		Config:  cfg,
	}, nil
}
