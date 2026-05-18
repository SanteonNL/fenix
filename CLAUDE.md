# FENIX — Claude Context

## Related Repositories

### HipsETL
**Path:** `C:\Users\t.hetterscheid\Repo\HipsETL`

HipsETL is the ETL pipeline that feeds data into HIPS (Health Information Platform Services) — the primary consumer of FENIX's FHIR output. It is a Go application (`module dev.azure.com/SanteonNL/Santeon/_git/HIPSETL`) organized as multiple apps:

- `datasetGenerator` — core ETL pipeline (DSG/ETL1), transforms source data into datasets
- `goUtilities` — shared utilities, including `PreLoadRecords` for API pre-loading
- `fhirApi` — FHIR API interactions
- `dataverificatie` — data verification tooling
- `apiExtractor` — API extraction logic

Key architectural concepts in HipsETL:
- **DSG (Dataset Generator)**: the main ETL pipeline that processes SQL/API sources into output datasets
- **API providers**: plugin-based system (Luscii, Castor) that pre-loads API data into SQLite before the standard ETL pipeline runs
- **In-memory SQLite staging**: API data is fetched and staged in SQLite so the existing SQL-based ETL pipeline can consume it unchanged
- Datasources are configured via YAML + environment variables; output is parquet/CSV files consumed downstream by HIPS
