# FENIX — Claude Context

## Rules

- **Config schema must stay in sync.** `fenix-config.schema.json` is generated from the config structs in `config/` (including their doc comments, which become the editor hover text). After changing a config struct, field, struct tag, or *only a field description*, run `make schema` and commit the regenerated schema in the same commit.
- Config field descriptions are written in Dutch (they are read by Dutch users) and start with "Verplicht" or "Optioneel".

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
