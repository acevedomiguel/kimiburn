package kimiburn

import (
	"io"

	"github.com/acevedomiguel/kimiburn/internal/app"
)

// PublicSchemaVersion is the stable public JSON schema version. It is
// independent of the StoredState and snapshot versions.
const PublicSchemaVersion = app.PublicSchemaVersion

// JSONOptions supplies transport-independent output metadata.
type JSONOptions = app.ProjectionOptions

// WriteJSON emits one indented schema-1 document and a trailing newline to the
// caller-selected writer.
func WriteJSON(w io.Writer, snapshot Snapshot, opts JSONOptions) error {
	return app.WriteJSON(w, snapshot, opts)
}
