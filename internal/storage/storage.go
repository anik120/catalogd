package storage

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"os"
)

// Instance is a storage instance that stores FBC content of catalogs
// added to a cluster. It can be used to Store or Delete FBC in the
// host's filesystem. It also a manager runnable object, that starts
// a server to serve the content stored.
type Instance interface {
	Store(ctx context.Context, catalog string, fsys fs.FS) error
	Delete(catalog string) error
	ContentExists(catalog string) bool

	BaseURL(catalog string) string
	StorageServerHandler() http.Handler
}

type Handler interface {
	Prepare(rootDir string, r io.Reader) error
	Path() string
	HandlerForCatalog(catalogName string, catalogFile *os.File) http.Handler
	ContentExists(catalog string) bool
}
