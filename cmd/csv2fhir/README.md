# CSV to FHIR Converter

A command-line tool to load CSV files into a database and intelligently convert them to FHIR resources using automatic column name mapping.

## Features

- **Multiple Database Support**: SQLite (default), PostgreSQL, MySQL
- **CSV Loading**: Automatic table creation and data import
- **Intelligent FHIR Conversion**: Automatically derives FHIR field mappings from SQL column naming conventions
- **Flexible Configuration**: YAML-based configuration for database, CSV, and FHIR settings
- **Output Formats**: JSON and NDJSON export
- **Zero Configuration Mapping**: Just name your CSV columns intelligently and conversion happens automatically

## Quick Start

### 1. Configuration

Create or edit `config/csv2fhir.yaml`:

```yaml
database:
  type: sqlite
  path: data/csv2fhir.db

csv:
  inputDir: data/csv
  delimiter: ","
  hasHeader: true

fhir:
  resourceType: Patient
  
output:
  dir: output
  format: json
```

### 2. Prepare CSV Files

Place your CSV files in the `data/csv` directory with intelligent column names:

**Example: `data/csv/patients.csv`**
```csv
id,first_name,last_name,date_of_birth,gender,email,phone_number,address,city,postal_code
1,John,Doe,1990-01-15,male,john@example.com,555-1234,123 Main St,Springfield,12345
2,Jane,Smith,1985-06-20,female,jane@example.com,555-5678,456 Oak Ave,Springfield,12345
```

**Example: `data/csv/observations.csv`**
```csv
id,patient_id,code,loinc_code,value,unit,effective_date,status
1,1,Blood Pressure,8480-6,140,mmHg,2024-01-15,final
2,1,Body Weight,29463-7,75,kg,2024-01-15,final
```

### 3. Run the Tool

```bash
# Load CSV and convert to FHIR
go run ./cmd/csv2fhir -config config/csv2fhir.yaml -cmd all

# Only load CSV
go run ./cmd/csv2fhir -cmd load

# Only convert
go run ./cmd/csv2fhir -cmd convert
```

## Intelligent Column Naming

The converter automatically detects column names and maps them to FHIR fields. No configuration needed!

### Patient Resource

| CSV Column | FHIR Field | Example |
|------------|-----------|---------|
| `id` | Patient.id | `123` |
| `first_name` / `given_name` | Patient.name[0].given[0] | `John` |
| `last_name` / `family_name` | Patient.name[0].family | `Doe` |
| `date_of_birth` / `birth_date` / `dob` | Patient.birthDate | `1990-01-15` |
| `gender` / `sex` | Patient.gender | `male`, `female` |
| `email` | Patient.telecom[0].value | `john@example.com` |
| `phone_number` / `phone` | Patient.telecom[1].value | `555-1234` |
| `address` | Patient.address[0].text | `123 Main St` |
| `city` | Patient.address[0].city | `Springfield` |
| `postal_code` | Patient.address[0].postalCode | `12345` |
| `country` | Patient.address[0].country | `USA` |
| `bsn` / `ssn` | Patient.identifier[0].value | `123456789` |
| `managing_organization` | Patient.managingOrganization.reference | `Org/456` |
| `general_practitioner` / `gp` | Patient.generalPractitioner[0].reference | `Practitioner/789` |
| `marital_status` | Patient.maritalStatus.coding[0].code | `M`, `S`, `D` |
| `active` / `is_active` | Patient.active | `true`, `1`, `yes` |

### Observation Resource

| CSV Column | FHIR Field | Notes |
|------------|-----------|-------|
| `id` | Observation.id | |
| `code` / `loinc_code` | Observation.code.coding[0].code | Uses LOINC code system |
| `value` | Observation.value.value | Numeric value |
| `unit` / `units` | Observation.value.unit | e.g., `mg/dL`, `mmHg` |
| `value_string` | Observation.value.valueString | String value |
| `effective_date` / `effective_time` | Observation.effectiveDateTime | Date value |
| `status` | Observation.status | `final`, `preliminary`, `amended` |
| `reference_range_low` | Observation.referenceRange[0].low.value | Normal range low |
| `reference_range_high` | Observation.referenceRange[0].high.value | Normal range high |
| `interpretation` | Observation.interpretation[0].coding[0].code | `H`, `L`, `N` |
| `method` | Observation.method.text | How measured |
| `performer` | Observation.performer[0].reference | Who performed |
| `patient` / `subject` | Observation.subject.reference | Patient reference |
| `encounter` | Observation.encounter.reference | Encounter reference |
| `category` | Observation.category[0].coding[0].code | `vital-signs`, `laboratory` |

### Condition Resource

| CSV Column | FHIR Field | Notes |
|------------|-----------|-------|
| `id` | Condition.id | |
| `code` / `snomed_code` / `icd_code` | Condition.code.coding[0].code | SNOMED CT or ICD-10 |
| `diagnosis` / `diagnosis_name` | Condition.code.text | Human-readable name |
| `clinical_status` / `status` | Condition.clinicalStatus.coding[0].code | `active`, `recurrence`, `remission` |
| `verification_status` | Condition.verificationStatus.coding[0].code | `confirmed`, `unconfirmed`, `refuted` |
| `severity` | Condition.severity.coding[0].code | `mild`, `moderate`, `severe` |
| `onset_date` / `start_date` | Condition.onsetDateTime | When started |
| `abatement_date` / `end_date` | Condition.abatementDateTime | When ended |
| `recorded_date` | Condition.recordedDate | When recorded |
| `patient` / `subject` | Condition.subject.reference | Patient reference |
| `encounter` | Condition.encounter.reference | Encounter reference |

### Procedure Resource

| CSV Column | FHIR Field | Notes |
|------------|-----------|-------|
| `id` | Procedure.id | |
| `code` / `snomed_code` | Procedure.code.coding[0].code | SNOMED CT code |
| `procedure_type` / `description` | Procedure.code.text | Human-readable name |
| `status` | Procedure.status | `preparation`, `in-progress`, `completed`, `cancelled` |
| `performed_date` / `procedure_date` / `date` | Procedure.performedDateTime | When performed |
| `performer` / `surgeon` | Procedure.performer[0].actor.reference | Who performed |
| `location` | Procedure.location.reference | Where performed |
| `outcome` | Procedure.outcome.coding[0].code | Result |
| `patient` / `subject` | Procedure.subject.reference | Patient reference |
| `encounter` | Procedure.encounter.reference | Encounter reference |
| `reason` / `reason_code` | Procedure.reasonCode[0].text | Why performed |
| `notes` | Procedure.note[0].text | Additional notes |

### Organization Resource

| CSV Column | FHIR Field | Notes |
|------------|-----------|-------|
| `id` | Organization.id | |
| `name` | Organization.name | Organization name |
| `alias` | Organization.alias[0] | Alternative name |
| `email` | Organization.telecom[0].value | Contact email |
| `phone` | Organization.telecom[1].value | Contact phone |
| `fax` | Organization.telecom[2].value | Fax number |
| `website` | Organization.telecom[3].value | Website URL |
| `address` | Organization.address[0].text | Full address |
| `street` | Organization.address[0].line[0] | Street address |
| `city` | Organization.address[0].city | City |
| `postal_code` | Organization.address[0].postalCode | Postal code |
| `country` | Organization.address[0].country | Country |
| `type` | Organization.type[0].coding[0].code | Organization type |
| `active` | Organization.active | Is active |

## Mapping Engine

The converter includes an intelligent `MappingEngine` that:

1. **Exact Matching**: Maps common column names directly to FHIR fields
2. **Normalized Matching**: Handles variations like `first_name` vs `firstName`
3. **Pattern Detection**: Recognizes column purposes by name patterns:
   - Date columns: `*_date`, `*_time`, `*_at`, `*born`, `dob`
   - Boolean columns: `active`, `is_*`, `has_*`, `deleted`, `enabled`
   - References: `*_id`, `*_ref`, `*_reference`, `patient`, `subject`, `performer`
   - Codes: `*code`, `*_type`, `*_status`, `category`, `classification`

4. **Code System Inference**: Automatically detects code systems:
   - `loinc_*` → LOINC
   - `snomed_*` → SNOMED CT
   - `icd_*` → ICD-10
   - `gender` / `sex` → Administrative Gender
   - `status` → Appropriate status code system

## Configuration

### Database Configuration

#### SQLite (Default)
```yaml
database:
  type: sqlite
  path: data/csv2fhir.db
```

#### PostgreSQL
```yaml
database:
  type: postgres
  connection: "postgres://user:password@localhost:5432/csv2fhir?sslmode=disable"
```

#### MySQL
```yaml
database:
  type: mysql
  connection: "user:password@tcp(localhost:3306)/csv2fhir"
```

### CSV Configuration

```yaml
csv:
  inputDir: data/csv        # Directory containing CSV files
  delimiter: ","            # CSV delimiter
  hasHeader: true           # Whether first row contains headers
```

### FHIR Configuration

```yaml
fhir:
  resourceType: Patient     # Resource type: Patient, Observation, Condition, Procedure, Organization
  mappings: ""              # Optional: path to custom column mapping file (JSON)
```

### Output Configuration

```yaml
output:
  dir: output               # Output directory
  format: json              # Output format: json or ndjson
```

## Command-Line Options

```
-config string
    Path to configuration file (default "config/csv2fhir.yaml")

-file string
    Specific CSV file to load (optional, loads all if not specified)

-cmd string
    Command: load, convert, all (default "all")

-help
    Show help message
```

## Example Usage

```bash
# Load all CSV files and convert to FHIR (automatic mapping)
go run ./cmd/csv2fhir

# Load specific CSV file
go run ./cmd/csv2fhir -file patients.csv -cmd load

# Convert existing tables to FHIR
go run ./cmd/csv2fhir -cmd convert

# Use custom configuration
go run ./cmd/csv2fhir -config config/custom.yaml

# Verbose logging
DEBUG=true go run ./cmd/csv2fhir
```

## Output

The tool creates FHIR resources in the specified output directory:

```
output/
├── patients.json          # Converted Patient resources
├── observations.json      # Converted Observation resources
├── conditions.json        # Converted Condition resources
├── procedures.json        # Converted Procedure resources
└── organizations.json     # Converted Organization resources
```

Each file contains FHIR resources in the specified format (JSON or NDJSON).

## Logging

The tool uses structured logging (zerolog) and outputs logs to stdout.

Example log output:
```
{"level":"info","time":"2024-04-07T10:30:00Z","message":"Configuration loaded","config":"config/csv2fhir.yaml"}
{"level":"info","time":"2024-04-07T10:30:01Z","message":"Connected to SQLite database","path":"data/csv2fhir.db"}
{"level":"info","time":"2024-04-07T10:30:02Z","message":"Loading CSV file","file":"data/csv/patients.csv","table":"patients"}
{"level":"info","time":"2024-04-07T10:30:02Z","message":"Derived FHIR mappings from columns","count":10}
{"level":"debug","time":"2024-04-07T10:30:02Z","message":"Mapping","column":"first_name","fhirPath":"Patient.name[0].given[0]","dataType":"string"}
```

## Troubleshooting

### No mappings derived
- Check that your CSV columns match the naming conventions
- Verify the resource type in configuration matches your data
- Check logs for column mapping attempts

### Empty output
- Verify CSV files were loaded correctly (`-cmd load` only)
- Check that rows exist in the database
- Verify resource type configuration

### Database connection error
- Verify database configuration in `config/csv2fhir.yaml`
- Ensure database is running and accessible
- Check connection string format

### CSV file not found
- Verify `csv.inputDir` in configuration
- Check that CSV files have `.csv` extension
- Ensure file paths are correct

## Development

To extend the converter:

1. **Add new resource type**: 
   - Add mappings in `mapping.go`
   - Implement `convertTo<ResourceType>` method in `converter.go`

2. **Custom mappings**: 
   - Edit mappings in `NewMappingEngine()` function
   - Add new pattern detection in `inferMapping()` method

3. **New database type**: 
   - Add case in `initializeDatabase` function in `main.go`

## Architecture

### Components

1. **loader.go**: CSV loading with automatic table creation
2. **converter.go**: FHIR resource creation with intelligent field setting
3. **mapping.go**: Intelligent column-to-FHIR field mapping engine
4. **config.go**: Configuration management

### Data Flow

```
CSV File
   ↓
CSV Loader (loader.go) → Database Tables
   ↓
Table Data
   ↓
Mapping Engine (mapping.go) → Derived FHIR Mappings
   ↓
FHIR Converter (converter.go) → FHIR Resources
   ↓
JSON/NDJSON Export
```

## License

[Your License Here]
