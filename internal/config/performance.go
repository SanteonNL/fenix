package config

import (
	"runtime"
	"time"

	"github.com/jmoiron/sqlx"
)

// PoolConfig is the connection pool of one database: the staging database
// (staging.pool) or a source the server queries directly (sources.<name>.pool).
// Every field is optional; 0 (or omitted) means the default described per field.
type PoolConfig struct {
	MaxOpenConns    int           `yaml:"maxOpenConns"`    // default: 16 for SQL Server/Postgres, GOMAXPROCS for a SQLite file, 1 for in-memory SQLite
	MaxIdleConns    int           `yaml:"maxIdleConns"`    // default: MaxOpenConns, so bursts do not pay for reconnecting
	ConnMaxLifetime time.Duration `yaml:"connMaxLifetime"` // default: 30m (never for in-memory SQLite, which would lose its data)
	ConnMaxIdleTime time.Duration `yaml:"connMaxIdleTime"` // default: 5m  (never for in-memory SQLite)
}

// PerformanceConfig tunes the FHIR API server's use of CPUs and its export
// batching. These are not tied to one database connection, so they live at the
// top level; the connection pools are configured on the databases themselves.
type PerformanceConfig struct {
	// Conversion of rows to FHIR resources.
	StatementWorkers int `yaml:"statementWorkers"` // statements of one query run at once; default: the pool's MaxOpenConns
	BuildWorkers     int `yaml:"buildWorkers"`     // goroutines building+validating resources; default: GOMAXPROCS

	// Group $export.
	ExportBatchSize int `yaml:"exportBatchSize"` // patients per export batch; default: 1000
	ExportWorkers   int `yaml:"exportWorkers"`   // export batches in flight at once; default: 4
}

// Default values for the settings above that are not derived from the database.
const (
	defaultSQLPoolSize     = 16
	defaultConnMaxLifetime = 30 * time.Minute
	defaultConnMaxIdleTime = 5 * time.Minute
	defaultExportBatchSize = 1000
	defaultExportWorkers   = 4
)

// ResolvedPool is the effective pool setting after defaults.
type ResolvedPool struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// Resolve fills in the defaults for a database of kind driver ("sqlserver",
// "postgres", "sqlite", ...). inMemory marks an in-memory SQLite database,
// which only exists on a single connection and so cannot be pooled.
func (p PoolConfig) Resolve(driver string, inMemory bool) ResolvedPool {
	if inMemory {
		// Every extra connection would open its own, empty, database; and a
		// connection that gets closed takes the data with it.
		return ResolvedPool{MaxOpenConns: 1, MaxIdleConns: 1}
	}

	r := ResolvedPool(p)
	if r.MaxOpenConns <= 0 {
		if driver == "sqlserver" || driver == "postgres" {
			r.MaxOpenConns = defaultSQLPoolSize
		} else {
			r.MaxOpenConns = runtime.GOMAXPROCS(0)
		}
	}
	if r.MaxIdleConns <= 0 || r.MaxIdleConns > r.MaxOpenConns {
		r.MaxIdleConns = r.MaxOpenConns
	}
	if r.ConnMaxLifetime <= 0 {
		r.ConnMaxLifetime = defaultConnMaxLifetime
	}
	if r.ConnMaxIdleTime <= 0 {
		r.ConnMaxIdleTime = defaultConnMaxIdleTime
	}
	return r
}

// Apply configures db's connection pool and returns the resolved setting. It
// must run before db is used concurrently.
func (p PoolConfig) Apply(db *sqlx.DB, inMemory bool) ResolvedPool {
	r := p.Resolve(db.DriverName(), inMemory)
	db.SetMaxOpenConns(r.MaxOpenConns)
	db.SetMaxIdleConns(r.MaxIdleConns)
	db.SetConnMaxLifetime(r.ConnMaxLifetime)
	db.SetConnMaxIdleTime(r.ConnMaxIdleTime)
	return r
}

// EffectiveStatementWorkers is how many statements of one query may run at
// once on a database with the given resolved pool: the configured value, but
// never more than the pool has connections (extra ones would only queue).
func (p PerformanceConfig) EffectiveStatementWorkers(pool ResolvedPool) int {
	if p.StatementWorkers > 0 && p.StatementWorkers < pool.MaxOpenConns {
		return p.StatementWorkers
	}
	return pool.MaxOpenConns
}

// ExportBatch is the number of patients per export batch.
func (p PerformanceConfig) ExportBatch() int {
	if p.ExportBatchSize > 0 {
		return p.ExportBatchSize
	}
	return defaultExportBatchSize
}

// ExportParallelism is the number of export batches in flight at once.
func (p PerformanceConfig) ExportParallelism() int {
	if p.ExportWorkers > 0 {
		return p.ExportWorkers
	}
	return defaultExportWorkers
}

// IsInMemorySQLite reports whether the staging database is an in-memory SQLite database.
func (sc *StagingConfig) IsInMemorySQLite() bool {
	return (sc.Database == "" || sc.Database == "sqlite") && sc.StagingPath() == ":memory:"
}
