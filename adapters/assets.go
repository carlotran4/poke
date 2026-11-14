package adapters

import _ "embed"

// PiExtension runs inside the existing Pi process and preserves typed drafts.
//
//go:embed pi.mjs
var PiExtension string
