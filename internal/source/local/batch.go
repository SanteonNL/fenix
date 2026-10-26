package local

import (
	"database/sql"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

// insertBatchSize is the number of rows written per transaction, and the interval between
// progress log lines. One transaction per row is orders of magnitude slower on SQLite.
const insertBatchSize = 10000

// execer is satisfied by both *sqlx.DB and *sqlx.Tx.
type execer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Rebind(query string) string
}

// writeBatch writes rows in a single transaction. If any row or the commit fails, the
// transaction is rolled back and every row is retried on its own outside a transaction, so a
// bad row is reported individually through onRowErr and does not abort the others
// (PostgreSQL would otherwise abort the whole transaction on the first failed statement).
func writeBatch(
	db *sqlx.DB,
	table string,
	log zerolog.Logger,
	rows []map[string]interface{},
	write func(execer, map[string]interface{}) error,
	onRowErr func(map[string]interface{}, error),
) {
	err := func() error {
		tx, err := db.Beginx()
		if err != nil {
			return err
		}
		for _, row := range rows {
			if err := write(tx, row); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		return tx.Commit()
	}()
	if err == nil {
		return
	}
	log.Warn().Err(err).Str("table", table).Int("rows", len(rows)).Msg("batch write failed, retrying row by row")
	for _, row := range rows {
		if err := write(db, row); err != nil {
			onRowErr(row, err)
		}
	}
}
