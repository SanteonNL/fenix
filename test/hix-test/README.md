# HIX-Test SQL Server Setup

This directory contains Docker Compose configuration and SQL initialization scripts for running a mock HIX (Hospital Information System) SQL Server database for testing purposes.

## Overview

The hix-test source is a SQL Server-based data source that demonstrates:
- **Staging queries**: SQL queries that read from the external SQL Server and load data into the fenix staging database
- **Data consolidation**: BSN mapping tables that enable combining data from multiple sources (e.g., linking HIX and Luscii patient records)
- **FHIR conversion**: SQL queries that transform staging data into FHIR resources

## Quick Start

### 1. Start the SQL Server

```bash
cd test/hix-test
docker-compose up -d
```

This starts a SQL Server Express instance with:
- **Container**: `hix-test-sqlserver`
- **Port**: 1433
- **Database**: `hix_test`
- **Username**: `sa`
- **Password**: `YourStrongPassword123!`

Wait for the health check to pass (~30 seconds):
```bash
docker-compose logs -f sqlserver
```

### 2. Verify the Database

Connect using any SQL client:

```
Server:   localhost
Port:     1433
Username: sa
Password: YourStrongPassword123!
Database: hix_test
```

Or test with `sqlcmd` (if installed):
```bash
sqlcmd -S localhost -U sa -P "YourStrongPassword123!" -d hix_test -Q "SELECT COUNT(*) FROM patients"
```

### 3. Run the CSV to FHIR converter

From the repository root:

```bash
go run ./cmd/csv2fhir -cmd all
```

Or with explicit config:
```bash
go run ./cmd/csv2fhir -config config/csv2fhir.yaml -cmd all
```

This will:
1. Load data from the SQL Server into the staging database
2. Execute FHIR conversion queries
3. Output FHIR resources to `output/`

### 4. Stop the SQL Server

```bash
cd test/hix-test
docker-compose down
```

To also remove the data volume:
```bash
docker-compose down -v
```

## Architecture

### Schema

The `hix_test` database contains raw HIX tables:

- **patients**: Core patient demographics
- **patient_names**: Patient name components
- **patient_addresses**: Patient addresses
- **patient_telecom**: Patient contact information (phone, email)
- **patient_identifiers**: Patient identifiers including BSN
- **encounters**: Healthcare encounters/visits
- **observations**: Clinical observations (vital signs, lab results)
- **bsn_mapping**: Mapping table for data consolidation across sources

### Data Flow

```
SQL Server (hix_test)
    ↓
[Staging Queries: queries/hix-test/staging/*.sql]
    ↓
Fenix Staging Database (SQLite or PostgreSQL)
    ↓
[FHIR Conversion Queries: queries/hix-test/*.sql]
    ↓
FHIR Resources (output/)
```

### Configuration

The hix-test source is configured in `config/csv2fhir.yaml`:

```yaml
sources:
  hix-test:
    type: sqlserver
    connection_string: "server=localhost;user id=sa;password=YourStrongPassword123!;port=1433;database=hix_test"
    staging_dir: "queries/hix-test/staging"
```

## Staging Queries

Staging queries are SQL scripts in `queries/hix-test/staging/` that read from the raw SQL Server and load normalized data into the staging database. Each file is named after the table it creates:

- **patients.sql**: Loads patient demographics
- **patient_names.sql**: Loads patient names
- **patient_addresses.sql**: Loads addresses
- **patient_telecom.sql**: Loads contact info
- **patient_identifiers.sql**: Loads identifiers
- **bsn_mapping.sql**: Loads BSN mapping (for data consolidation)
- **encounters.sql**: Loads encounters
- **observations.sql**: Loads observations

### Data Consolidation with BSN

The `bsn_mapping` table enables linking HIX data with data from other sources (e.g., Luscii):

```sql
SELECT * FROM bsn_mapping;

mapping_id | hix_patient_number | bsn       | luscii_patient_id
-----------|-------------------|-----------|------------------
1          | HIX001            | 123456789 | luscii_patient_1
2          | HIX002            | 987654321 | luscii_patient_2
...
```

This allows queries to join HIX and Luscii data by BSN, consolidating patient records across multiple sources.

## FHIR Conversion Queries

FHIR conversion queries in `queries/hix-test/` transform staging data into FHIR resources:

- **patient.sql**: Converts to FHIR Patient resources
- **encounter.sql**: Converts to FHIR Encounter resources
- **observation.sql**: Converts to FHIR Observation resources

These queries use raw column names from the staging tables and map them to FHIR paths.

## Sample Data

The `init.sql` script creates and populates the database with sample data:

- **5 patients** with demographics, names, addresses, telecom, and identifiers (including BSN)
- **5 encounters** (inpatient and outpatient)
- **8 observations** (blood pressure, weight, glucose levels)
- **BSN mapping** linking HIX patients to Luscii patient IDs where applicable

## Extending the Schema

To add new tables or columns:

1. Update `init.sql` to create the new schema and insert sample data
2. Create corresponding staging SQL query files in `queries/hix-test/staging/`
3. Update FHIR conversion queries in `queries/hix-test/` if needed
4. Restart the container:
   ```bash
   docker-compose down -v
   docker-compose up -d
   ```

## Troubleshooting

### Connection Refused
If you get "Connection refused", ensure the container is running:
```bash
docker-compose ps
```

If the container is not running, check the logs:
```bash
docker-compose logs sqlserver
```

### Database Already Exists
If you get "Database already exists", remove the volume:
```bash
docker-compose down -v
docker-compose up -d
```

### Staging Query Fails
If a staging query fails, check:
1. The SQL Server is running and accessible
2. The table name in the SQL file matches the `.sql` filename
3. The connection string in `csv2fhir.yaml` is correct

## Performance Notes

- The sample data is minimal for testing purposes
- For large datasets, consider indexing key columns (patient_id, bsn, hix_patient_number)
- The staging tables use TEXT columns for simplicity; consider VARCHAR/INT for production

## Next Steps

1. **Real Data Integration**: Replace `init.sql` with actual HIX database schema and data
2. **Advanced Consolidation**: Enhance `bsn_mapping` with additional matching rules (e.g., name + DOB)
3. **Production Deployment**: Update connection string to point to real HIX database
4. **Performance Tuning**: Add indexes and optimize queries for large datasets
