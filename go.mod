module github.com/Molecule-AI/molecule-ai-plugin-gh-identity

go 1.25.0

require gopkg.in/yaml.v3 v3.0.1

// This plugin's Mutator type satisfies monorepo's provisionhook.EnvMutator
// structurally — we don't import it, so no cross-module replace directive
// is needed. If we ever need to reference exported types from
// molecule-monorepo/platform, uncomment:
//
//   replace github.com/Molecule-AI/molecule-monorepo/platform => ../molecule-monorepo/workspace-server
//
// Keeping this out of the require list lets the plugin build standalone in CI
// without checking out the monorepo.
