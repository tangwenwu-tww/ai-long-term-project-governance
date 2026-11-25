// Package v0 exposes the exact contract bytes shipped by the CLI.
package v0

import _ "embed"

var (
	//go:embed install-manifest.schema.json
	installManifestSchema []byte
	//go:embed instance.schema.json
	instanceSchema []byte
)

// InstallManifestSchema returns a copy of the canonical install schema bytes.
func InstallManifestSchema() []byte { return append([]byte(nil), installManifestSchema...) }

// InstanceSchema returns a copy of the canonical instance schema bytes.
func InstanceSchema() []byte { return append([]byte(nil), instanceSchema...) }
