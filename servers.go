// Package servers represents the interface (and its implementations) of interaction with the server.
package servers

import (
	"context"
)

// Launcher delivers an interface to the server.
type Launcher interface {
	// Serve serving the server.
	Serve() error
	// Stop stops the server.
	Stop(ctx context.Context) error
}
