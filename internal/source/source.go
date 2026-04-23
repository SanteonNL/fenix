package source

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// Source loads data from an external system into the staging database.
// Each source corresponds to one entry under `sources:` in the config.
type Source interface {
	Load(ctx context.Context, db *sqlx.DB) error
}
