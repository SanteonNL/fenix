# Test Database Setup

This directory contains Docker configuration for running a SQL Server test database with pre-loaded test data.

## Quick Start

### Start the test database

```bash
cd test
docker-compose up -d
```

The first time you run this, it will:
1. Download the SQL Server Docker image
2. Start SQL Server
3. Create the `testdb` database
4. Run [setup.sql](data/sql/setup.sql) to populate test data

### Stop the test database

```bash
cd test
docker-compose down
```

### Reset the database (remove all data and restart)

```bash
cd test
docker-compose down -v
docker-compose up -d
```

## Database Details

- **Host:** localhost
- **Port:** 1433
- **Database:** testdb
- **Username:** sa
- **Password:** TestPass123!

## Configuration

Update your [development.config.yaml](../config/development.config.yaml) to use this test database:

```yaml
datasources:
  - name: test_db
    type: sql
    driver: sqlserver
    connection_string: "server=localhost;user id=sa;password=TestPass123!;port=1433;database=testdb;encrypt=disable"
```

## Test Data

The database is automatically initialized with test data from [setup.sql](data/sql/setup.sql) when the container first starts.

The test data includes:
- Patient records
- Names
- Practitioners
- Patient-Practitioner relationships
- Contacts and contact points
- Observations
- Encounters
- Questionnaires
- Couple/family relationships

## Troubleshooting

### Check if the container is running

```bash
docker ps
```

You should see `fenix-test-db` in the list.

### View logs

```bash
docker logs fenix-test-db
```

### Connect to the database manually

```bash
docker exec -it fenix-test-db /opt/mssql-tools/bin/sqlcmd -S localhost -U sa -P TestPass123! -d testdb
```

Then you can run SQL queries:
```sql
SELECT * FROM patient;
GO
```

Type `exit` to quit.
