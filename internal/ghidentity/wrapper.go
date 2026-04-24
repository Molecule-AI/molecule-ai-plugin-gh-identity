package ghidentity

import _ "embed"

// WrapperScript is the shell wrapper that replaces `gh` in the workspace
// container's PATH. Shipped to the workspace via env var
// MOLECULE_GH_WRAPPER_B64 (base64) — the template's install.sh decodes
// and writes it to /usr/local/bin/gh.
//
// Embedded (not a constant) so gofmt/go vet treat it like source; easier
// to edit than a multi-line Go string.
//
//go:embed wrapper.sh
var WrapperScript string
