# SQL to FHIR Conversion: Intelligent Naming Conventions Guide

## Executive Summary

This guide documents patterns and best practices for converting SQL data to FHIR resources using intelligent naming conventions. Rather than relying on hardcoded mappings, the converter can auto-derive FHIR field mappings from SQL column names by recognizing naming patterns and SQL column structure.

---

## 1. FHIR Resource Type Inference from Table Names

### Pattern Matching Rules

**Convention**: SQL table names should indicate the FHIR resource type they contain.

#### Rule 1: Direct Resource Name Match
```
Table Name Pattern          → FHIR Resource Type
patients, patient_*         → Patient
observations, observation_* → Observation
conditions, condition_*     → Condition
procedures, procedure_*     → Procedure
organizations, org_*        → Organization
encounters, encounter_*     → Encounter
medications, medication_*   → Medication
```

#### Rule 2: Camel Case to FHIR Resource
```
Table: PatientDemographics   → Patient
Table: ObservationResults    → Observation
Table: ProcedureNotes        → Procedure
Table: ConditionDiagnosis    → Condition
```

#### Rule 3: Plural Forms
```
patients (plural)   → Patient (singular) resource type
```

#### Rule 4: Prefix Matching (Entity Types)
```
Prefix          → Resource Type
pt_*            → Patient
obs_*           → Observation
cond_*          → Condition
proc_*          → Procedure
org_*           → Organization
enc_*           → Encounter
med_*           → Medication
```

### Implementation Strategy

```go
func inferResourceTypeFromTableName(tableName string) string {
    tableLower := strings.ToLower(tableName)
    
    // Direct matches
    if strings.HasPrefix(tableLower, "patient") { return "Patient" }
    if strings.HasPrefix(tableLower, "observation") { return "Observation" }
    if strings.HasPrefix(tableLower, "condition") { return "Condition" }
    if strings.HasPrefix(tableLower, "procedure") { return "Procedure" }
    if strings.HasPrefix(tableLower, "organization") { return "Organization" }
    
    // Prefix matches
    if strings.HasPrefix(tableLower, "pt_") { return "Patient" }
    if strings.HasPrefix(tableLower, "obs_") { return "Observation" }
    if strings.HasPrefix(tableLower, "cond_") { return "Condition" }
    
    return "" // Unknown type
}
```

---

## 2. SQL Column to FHIR Field Mapping Conventions

### 2.1 Fundamental Field Mappings

#### Patient Resource Example

| SQL Column Pattern | FHIR Field | Type | Notes |
|---|---|---|---|
| `id`, `patient_id`, `pt_id` | `Patient.id` | String | Primary identifier |
| `first_name`, `fname` | `Patient.name[0].given` | String | Given name |
| `last_name`, `lname`, `family_name` | `Patient.name[0].family` | String | Family name |
| `full_name`, `name` | `Patient.name[0].text` | String | Complete name |
| `gender`, `sex`, `biological_sex` | `Patient.gender` | Code | `male\|female\|other\|unknown` |
| `date_of_birth`, `birth_date`, `dob`, `birthdate` | `Patient.birthDate` | Date | YYYY-MM-DD |
| `date_of_death`, `death_date`, `deceased_date` | `Patient.deceasedDateTime` | DateTime | YYYY-MM-DD[THH:mm:ss] |
| `active`, `is_active` | `Patient.active` | Boolean | true\|false |
| `email`, `email_address` | `Patient.telecom[].value` (system: email) | String | Contact info |
| `phone`, `telephone`, `phone_number` | `Patient.telecom[].value` (system: phone) | String | Contact info |
| `address`, `street_address` | `Patient.address[].line[]` | String | Street address |
| `city`, `municipality` | `Patient.address[].city` | String | City |
| `state`, `province`, `region` | `Patient.address[].state` | String | State/Province |
| `postal_code`, `zip_code`, `zipcode` | `Patient.address[].postalCode` | String | Postal code |
| `country` | `Patient.address[].country` | String | Country |
| `marital_status`, `marital_status_code` | `Patient.maritalStatus.text` | String | Marital status |
| `language`, `preferred_language` | `Patient.communication[].language.text` | String | Language spoken |
| `organization_id`, `managing_org`, `organization` | `Patient.managingOrganization.reference` | Reference | Organization reference |
| `general_practitioner_id`, `gp_id`, `provider_id` | `Patient.generalPractitioner[].reference` | Reference | Practitioner reference |

#### Observation Resource Example

| SQL Column Pattern | FHIR Field | Type | Notes |
|---|---|---|---|
| `id`, `observation_id`, `obs_id` | `Observation.id` | String | Primary identifier |
| `status`, `observation_status` | `Observation.status` | Code | `registered\|preliminary\|final\|amended` |
| `code`, `test_code`, `loinc_code` | `Observation.code.coding[].code` | String | What was observed |
| `code_display`, `test_name` | `Observation.code.text` | String | Display name |
| `code_system`, `code_system_url` | `Observation.code.coding[].system` | URI | Coding system (e.g., LOINC) |
| `value`, `result`, `numeric_value` | `Observation.value.value` | Numeric | Quantitative result |
| `value_unit`, `unit`, `measurement_unit` | `Observation.value.unit` | String | Unit of measurement |
| `value_code`, `result_code` | `Observation.valueCodeableConcept.coding[].code` | String | Coded result |
| `value_display`, `result_name` | `Observation.valueCodeableConcept.text` | String | Result display |
| `value_string` | `Observation.valueString` | String | Text result |
| `value_boolean` | `Observation.valueBoolean` | Boolean | Boolean result |
| `effective_date`, `observation_date`, `test_date` | `Observation.effectiveDateTime` | DateTime | When observed |
| `issued_date`, `result_date` | `Observation.issued` | DateTime | When reported |
| `subject_id`, `patient_id` | `Observation.subject.reference` | Reference | Patient reference |
| `performer_id`, `ordered_by` | `Observation.performer[].reference` | Reference | Who performed |
| `reference_range_low`, `normal_low` | `Observation.referenceRange[].low.value` | Numeric | Normal range low |
| `reference_range_high`, `normal_high` | `Observation.referenceRange[].high.value` | Numeric | Normal range high |
| `category`, `observation_category` | `Observation.category[].text` | String | Classification |
| `interpretation`, `flag` | `Observation.interpretation[].text` | String | Result interpretation |
| `note`, `comment`, `remarks` | `Observation.note[].text` | String | Narrative comment |

#### Condition Resource Example

| SQL Column Pattern | FHIR Field | Type | Notes |
|---|---|---|---|
| `id`, `condition_id` | `Condition.id` | String | Primary identifier |
| `status`, `clinical_status` | `Condition.clinicalStatus.coding[].code` | Code | `active\|recurrence\|relapse\|inactive\|remission\|resolved` |
| `verification_status` | `Condition.verificationStatus.coding[].code` | Code | `unconfirmed\|provisional\|differential\|confirmed\|refuted\|entered-in-error` |
| `code`, `diagnosis_code`, `icd_code` | `Condition.code.coding[].code` | String | Condition/diagnosis code |
| `code_display`, `diagnosis_name` | `Condition.code.text` | String | Condition display text |
| `code_system` | `Condition.code.coding[].system` | URI | Code system (ICD-10, SNOMED) |
| `category`, `condition_category` | `Condition.category[].text` | String | Category |
| `severity`, `condition_severity` | `Condition.severity.text` | String | Severity |
| `body_site`, `affected_site` | `Condition.bodySite[].text` | String | Body location |
| `subject_id`, `patient_id` | `Condition.subject.reference` | Reference | Patient reference |
| `encounter_id` | `Condition.encounter.reference` | Reference | Related encounter |
| `onset_date`, `start_date`, `diagnosis_date` | `Condition.onsetDateTime` | DateTime | When condition started |
| `abatement_date`, `end_date`, `resolved_date` | `Condition.abatementDateTime` | DateTime | When condition resolved |
| `recorded_date` | `Condition.recordedDate` | DateTime | When recorded |
| `recorder_id`, `recorded_by` | `Condition.recorder.reference` | Reference | Who recorded |
| `asserter_id`, `asserted_by` | `Condition.asserter.reference` | Reference | Who asserts |
| `note`, `comment` | `Condition.note[].text` | String | Comment |

#### Procedure Resource Example

| SQL Column Pattern | FHIR Field | Type | Notes |
|---|---|---|---|
| `id`, `procedure_id`, `proc_id` | `Procedure.id` | String | Primary identifier |
| `status` | `Procedure.status` | Code | `preparation\|in-progress\|suspended\|aborted\|completed\|entered-in-error` |
| `code`, `procedure_code` | `Procedure.code.coding[].code` | String | Procedure code |
| `code_display`, `procedure_name` | `Procedure.code.text` | String | Procedure display |
| `category`, `procedure_category` | `Procedure.category.text` | String | Type of procedure |
| `subject_id`, `patient_id` | `Procedure.subject.reference` | Reference | Patient reference |
| `encounter_id` | `Procedure.encounter.reference` | Reference | Related encounter |
| `performed_date`, `procedure_date` | `Procedure.performedDateTime` | DateTime | When performed |
| `performer_id`, `surgeon_id` | `Procedure.performer[].actor.reference` | Reference | Who performed |
| `location_id` | `Procedure.location.reference` | Reference | Where performed |
| `reason_code`, `indication_code` | `Procedure.reasonCode[].coding[].code` | String | Why performed |
| `body_site`, `site` | `Procedure.bodySite[].text` | String | Body location |
| `outcome`, `result` | `Procedure.outcome.text` | String | Outcome |
| `note`, `report` | `Procedure.note[].text` | String | Procedure report |

### 2.2 Naming Convention Patterns

#### Pattern 1: Suffix-Based Type Inference
```
Column Suffix       → FHIR Field Type
_date               → DateTime or Date
_code               → Coding system value
_id                 → Reference field
_count              → Integer
_amount             → Quantity
_percent, _pct      → Ratio/Decimal
_flag               → Boolean
_text, _name        → Human-readable text
```

#### Pattern 2: Prefix-Based Field Groups
```
Prefix              → Meaning
code_*              → Coding information (code, display, system)
value_*             → Observation value (quantity, code, string, boolean)
reference_*         → Reference fields
date_*              → Date/DateTime fields
normal_*            → Reference range
effective_*         → Timing information
```

#### Pattern 3: Array/Collection Indicators
```
Naming Convention           → Becomes Array in FHIR
column_name_1, _2, _3       → name[0], name[1], name[2]
multiple_column_name        → name[] (collection)
```

---

## 3. Data Type Mapping

### Type Detection from Column Names and Values

```go
type FieldTypeMapping struct {
    SQLType     string
    FHIRType    string
    Transform   string
}

// Mapping rules based on column name patterns and SQL data types
var typeMappings = map[string]FieldTypeMapping{
    // String types
    "TEXT":      {SQLType: "TEXT", FHIRType: "string"},
    "VARCHAR":   {SQLType: "VARCHAR", FHIRType: "string"},
    
    // Integer types
    "INTEGER":   {SQLType: "INTEGER", FHIRType: "integer"},
    "INT":       {SQLType: "INT", FHIRType: "integer"},
    
    // Float/Decimal types
    "FLOAT":     {SQLType: "FLOAT", FHIRType: "decimal"},
    "DECIMAL":   {SQLType: "DECIMAL", FHIRType: "decimal"},
    "NUMERIC":   {SQLType: "NUMERIC", FHIRType: "decimal"},
    
    // Boolean types
    "BOOLEAN":   {SQLType: "BOOLEAN", FHIRType: "boolean"},
    "BOOL":      {SQLType: "BOOL", FHIRType: "boolean"},
    
    // Date types
    "DATE":      {SQLType: "DATE", FHIRType: "date"},
    "TIMESTAMP": {SQLType: "TIMESTAMP", FHIRType: "dateTime"},
    "DATETIME":  {SQLType: "DATETIME", FHIRType: "dateTime"},
    "DATETIME2": {SQLType: "DATETIME2", FHIRType: "dateTime"},
}
```

### Smart Type Inference

```go
func inferFieldType(columnName string, sqlType string, sampleValue interface{}) string {
    columnLower := strings.ToLower(columnName)
    
    // Check column name patterns first
    if strings.Contains(columnLower, "_date") || 
       strings.Contains(columnLower, "_datetime") ||
       strings.Contains(columnLower, "date_") {
        return "date"
    }
    
    if strings.Contains(columnLower, "_code") || 
       strings.Contains(columnLower, "code_") {
        return "code"
    }
    
    if strings.Contains(columnLower, "_id") && 
       !strings.Contains(columnLower, "patient_id") {
        return "reference"
    }
    
    if strings.Contains(columnLower, "_flag") || 
       strings.Contains(columnLower, "is_") ||
       strings.Contains(columnLower, "active") {
        return "boolean"
    }
    
    // Fall back to SQL type inference
    return sqlTypeToFHIRType(sqlType)
}
```

---

## 4. Special Handling: Complex FHIR Types

### 4.1 CodeableConcept (Code + Display + System)

**Pattern**: Use underscore separators to group related fields

```
SQL Columns:                    FHIR Field
code, code_display, code_system → CodeableConcept {
    coding[0]: {
        code: code,
        display: code_display,
        system: code_system
    },
    text: code_display
}
```

**Naming Convention**:
```
{prefix}_code           → coding[].code
{prefix}_code_display   → text or coding[].display
{prefix}_code_system    → coding[].system
{prefix}_text           → text
{prefix}_coding_system  → coding[].system
```

**Examples**:
```sql
observation:
    - observation_code      → Observation.code.coding[].code
    - observation_display   → Observation.code.text
    - observation_system    → Observation.code.coding[].system

condition:
    - diagnosis_code        → Condition.code.coding[].code
    - diagnosis_display     → Condition.code.text
    - diagnosis_system      → Condition.code.coding[].system

status_code             → status.coding[].code
status_display          → status.text
```

### 4.2 Quantity (Value + Unit + System + Code)

**Pattern**: Quantity fields contain numeric values with units

```
SQL Columns:                          FHIR Field
value, unit                           → Quantity {
    value: value,
    unit: unit,
    system: unit_system,
    code: unit_code
}
```

**Naming Convention**:
```
value               → value
value_unit          → unit
unit                → unit
measurement_unit    → unit
value_system        → system (UCUM)
value_code          → code (UCUM code)
```

**Examples**:
```sql
-- Observation with measurement
observation_value               → valueQuantity.value
observation_unit                → valueQuantity.unit ("mg")
observation_unit_system         → valueQuantity.system ("http://unitsofmeasure.org")
observation_unit_code           → valueQuantity.code ("mg")

-- Reference range
normal_low                       → referenceRange[].low.value
normal_high                      → referenceRange[].high.value
normal_unit                      → referenceRange[].low.unit
```

### 4.3 Reference (Links to Other Resources)

**Pattern**: Foreign keys become references

```
SQL Columns:                    FHIR Field
{entity}_id                     → Reference {
    reference: "{ResourceType}/{id}",
    type: "{ResourceType}"
}
```

**Naming Convention**:
```
patient_id                      → subject (for Patient)
subject_id                      → subject
performer_id                    → performer[]
recorder_id                     → recorder
asserter_id                     → asserter
organization_id                 → organization or managingOrganization
encounter_id                    → encounter
practitioner_id                 → performer[] or recorder
location_id                     → location
specimen_id                     → specimen
device_id                       → device
```

**Resolution Strategy**:
```go
func createReference(columnName string, idValue string) Reference {
    // Infer resource type from column name
    resourceType := inferResourceTypeFromColumnName(columnName)
    
    return Reference{
        Reference: fmt.Sprintf("%s/%s", resourceType, idValue),
        Type:      resourceType,
    }
}

func inferResourceTypeFromColumnName(columnName string) string {
    columnLower := strings.ToLower(columnName)
    
    if strings.Contains(columnLower, "patient") { return "Patient" }
    if strings.Contains(columnLower, "practitioner") { return "Practitioner" }
    if strings.Contains(columnLower, "organization") { return "Organization" }
    if strings.Contains(columnLower, "encounter") { return "Encounter" }
    if strings.Contains(columnLower, "location") { return "Location" }
    if strings.Contains(columnLower, "specimen") { return "Specimen" }
    if strings.Contains(columnLower, "device") { return "Device" }
    if strings.Contains(columnLower, "performer") { return "Practitioner" }
    if strings.Contains(columnLower, "recorder") { return "Practitioner" }
    
    return "Resource" // Generic fallback
}
```

### 4.4 Identifier (System + Value)

**Pattern**: Multiple identifier types with system and value

```
SQL Columns:                    FHIR Field
mrn, mrn_system                 → Identifier {
    system: "http://hospital.org/mrn",
    value: mrn
}

ssn, ssn_system                 → Identifier {
    system: "http://ssa.gov/ssn",
    value: ssn
}
```

**Naming Convention**:
```
{identifier_type}_value         → identifier[].value
{identifier_type}_system        → identifier[].system
{identifier_type}_use           → identifier[].use
mrn                             → Identifier with system="MRN"
ssn                             → Identifier with system="SSN"
national_id                     → Identifier with system="NDIN"
passport                        → Identifier with system="PASSPORT"
driver_license                  → Identifier with system="DL"
```

**Examples**:
```sql
medical_record_number           → Identifier { system: "MRN", value: ... }
social_security_number          → Identifier { system: "SSN", value: ... }
patient_identifier_system       → identifier[].system
patient_identifier_value        → identifier[].value
```

### 4.5 Period (Start + End)

**Pattern**: Date range fields

```
SQL Columns:                    FHIR Field
start_date, end_date            → Period {
    start: start_date,
    end: end_date
}
```

**Naming Convention**:
```
*_start_date / *_from           → start
*_end_date / *_to               → end
*_start / *_end                 → start/end
effective_start / effective_end  → Period
validity_start / validity_end   → Period
```

**Examples**:
```sql
validity_start, validity_end    → period { start, end }
insurance_start, insurance_end  → coverage.period
employment_start, employment_end → Related to practitioner role
```

### 4.6 HumanName (Given + Family + Prefix + Suffix)

**Pattern**: Name components

```
SQL Columns:                    FHIR Field
first_name, last_name           → HumanName {
    given: [first_name],
    family: last_name,
    text: full_name
}
```

**Naming Convention**:
```
first_name, given_name          → given[]
last_name, family_name          → family
middle_name                     → given[] (additional)
name_prefix, title              → prefix[]
name_suffix                     → suffix[]
full_name, name                 → text
display_name                    → text
```

**Examples**:
```sql
patient:
    - first_name            → name[0].given[0]
    - last_name             → name[0].family
    - middle_name           → name[0].given[1]
    - name_prefix (Dr., Mr.) → name[0].prefix[]
    - full_name             → name[0].text
```

### 4.7 Address Fields

**Pattern**: Address components

```
SQL Columns:                    FHIR Field
street, city, state, postal_code
                                → Address {
                                    line: [street],
                                    city: city,
                                    state: state,
                                    postalCode: postal_code,
                                    country: country
                                }
```

**Naming Convention**:
```
street, street_address, address → line[]
address_line_1, address_line_2  → line[]
city, municipality              → city
state, province, region         → state
postal_code, zip_code, zipcode  → postalCode
country                         → country
address_type                    → use (home|work|temp|old)
```

### 4.8 ContactPoint (Phone, Email, etc.)

**Pattern**: Multiple contact methods with types

```
SQL Columns:                    FHIR Field
phone, email, fax               → ContactPoint[] {
                                    system: "phone|email|fax|...",
                                    value: ...
                                }
```

**Naming Convention**:
```
phone, telephone, phone_number  → ContactPoint { system: "phone", value: ... }
email, email_address            → ContactPoint { system: "email", value: ... }
fax, fax_number                 → ContactPoint { system: "fax", value: ... }
website, url                    → ContactPoint { system: "url", value: ... }
{contact_type}_{format}         → ContactPoint array
```

---

## 5. Practical Examples: SQL to FHIR Conversions

### Example 1: Patient Table to Patient Resource

**SQL Table**:
```sql
CREATE TABLE patients (
    patient_id VARCHAR(20) PRIMARY KEY,
    first_name VARCHAR(50),
    last_name VARCHAR(50),
    date_of_birth DATE,
    gender VARCHAR(20),
    phone_number VARCHAR(20),
    email_address VARCHAR(100),
    active BOOLEAN,
    managing_organization_id VARCHAR(50)
);
```

**Auto-Derived Mapping**:
```json
{
    "patient_id": "Patient.id",
    "first_name": "Patient.name[0].given[0]",
    "last_name": "Patient.name[0].family",
    "date_of_birth": "Patient.birthDate",
    "gender": "Patient.gender",
    "phone_number": "Patient.telecom[0].value (system: phone)",
    "email_address": "Patient.telecom[1].value (system: email)",
    "active": "Patient.active",
    "managing_organization_id": "Patient.managingOrganization.reference (Organization)"
}
```

**Generated FHIR Resource**:
```json
{
    "resourceType": "Patient",
    "id": "PT-001",
    "name": [{
        "given": ["John"],
        "family": "Doe",
        "text": "John Doe"
    }],
    "birthDate": "1990-01-15",
    "gender": "male",
    "telecom": [
        {"system": "phone", "value": "555-1234"},
        {"system": "email", "value": "john@example.com"}
    ],
    "active": true,
    "managingOrganization": {
        "reference": "Organization/ORG-001"
    }
}
```

### Example 2: Observation Results Table to Observation Resource

**SQL Table**:
```sql
CREATE TABLE lab_results (
    result_id VARCHAR(20) PRIMARY KEY,
    patient_id VARCHAR(20),
    test_code VARCHAR(20),
    test_name VARCHAR(100),
    test_system VARCHAR(100),
    result_value DECIMAL(10,2),
    result_unit VARCHAR(20),
    unit_system VARCHAR(100),
    test_date TIMESTAMP,
    status VARCHAR(20),
    reference_low DECIMAL(10,2),
    reference_high DECIMAL(10,2)
);
```

**Auto-Derived Mapping**:
```json
{
    "result_id": "Observation.id",
    "patient_id": "Observation.subject.reference (Patient)",
    "test_code": "Observation.code.coding[0].code",
    "test_name": "Observation.code.text",
    "test_system": "Observation.code.coding[0].system",
    "result_value": "Observation.valueQuantity.value",
    "result_unit": "Observation.valueQuantity.unit",
    "unit_system": "Observation.valueQuantity.system",
    "test_date": "Observation.effectiveDateTime",
    "status": "Observation.status",
    "reference_low": "Observation.referenceRange[0].low.value",
    "reference_high": "Observation.referenceRange[0].high.value"
}
```

**Generated FHIR Resource**:
```json
{
    "resourceType": "Observation",
    "id": "OBS-001",
    "status": "final",
    "code": {
        "coding": [{
            "system": "http://loinc.org",
            "code": "2345-7",
            "display": "Glucose [mg/dL]"
        }],
        "text": "Glucose [mg/dL]"
    },
    "subject": {
        "reference": "Patient/PT-001"
    },
    "effectiveDateTime": "2024-01-15T09:30:00Z",
    "valueQuantity": {
        "value": 95.5,
        "unit": "mg/dL",
        "system": "http://unitsofmeasure.org",
        "code": "mg/dL"
    },
    "referenceRange": [{
        "low": {"value": 70},
        "high": {"value": 100}
    }]
}
```

### Example 3: Condition Diagnosis Table to Condition Resource

**SQL Table**:
```sql
CREATE TABLE diagnoses (
    condition_id VARCHAR(20) PRIMARY KEY,
    patient_id VARCHAR(20),
    icd_code VARCHAR(20),
    diagnosis_name VARCHAR(200),
    diagnosis_system VARCHAR(100),
    clinical_status VARCHAR(50),
    severity VARCHAR(50),
    diagnosis_date DATE,
    recorded_by VARCHAR(50)
);
```

**Auto-Derived Mapping**:
```json
{
    "condition_id": "Condition.id",
    "patient_id": "Condition.subject.reference (Patient)",
    "icd_code": "Condition.code.coding[0].code",
    "diagnosis_name": "Condition.code.text",
    "diagnosis_system": "Condition.code.coding[0].system",
    "clinical_status": "Condition.clinicalStatus.coding[0].code",
    "severity": "Condition.severity.text",
    "diagnosis_date": "Condition.onsetDateTime",
    "recorded_by": "Condition.recorder.reference (Practitioner)"
}
```

**Generated FHIR Resource**:
```json
{
    "resourceType": "Condition",
    "id": "COND-001",
    "code": {
        "coding": [{
            "system": "http://hl7.org/fhir/sid/icd-10-cm",
            "code": "E11.9",
            "display": "Type 2 diabetes mellitus without complications"
        }],
        "text": "Type 2 diabetes mellitus"
    },
    "subject": {
        "reference": "Patient/PT-001"
    },
    "clinicalStatus": {
        "coding": [{
            "system": "http://terminology.hl7.org/CodeSystem/condition-clinical",
            "code": "active"
        }],
        "text": "active"
    },
    "severity": {
        "text": "moderate"
    },
    "onsetDateTime": "2015-03-20",
    "recorder": {
        "reference": "Practitioner/PRAC-001"
    }
}
```

---

## 6. Implementation Strategy: Column Name Parser

### Algorithm: Extract FHIR Field Path from SQL Column Name

```go
type ColumnNameParser struct {
    columnName      string
    resourceType    string
    sqlType         string
    sampleValue     interface{}
}

func (p *ColumnNameParser) ParseToFHIRPath() string {
    // 1. Check predefined mappings first
    if mapping, exists := getPredefinedMapping(p.resourceType, p.columnName); exists {
        return mapping
    }
    
    // 2. Apply heuristic rules
    path := p.applyHeuristics()
    return path
}

func (p *ColumnNameParser) applyHeuristics() string {
    normalized := normalizeColumnName(p.columnName)
    tokens := tokenizeColumnName(normalized)
    
    // Build FHIR path based on tokens
    return p.buildFHIRPath(tokens)
}

func normalizeColumnName(name string) string {
    // Convert snake_case to camelCase
    // patient_id → patientId
    // first_name → firstName
    
    parts := strings.Split(strings.ToLower(name), "_")
    result := parts[0]
    for i := 1; i < len(parts); i++ {
        result += strings.Title(parts[i])
    }
    return result
}

func tokenizeColumnName(name string) []string {
    // Split on boundaries: camelCase → [camel, Case]
    // Identify token types: id, code, date, etc.
    
    var tokens []string
    var current string
    
    for i, r := range name {
        if unicode.IsUpper(r) && i > 0 {
            tokens = append(tokens, current)
            current = string(r)
        } else {
            current += string(r)
        }
    }
    if current != "" {
        tokens = append(tokens, current)
    }
    return tokens
}

func (p *ColumnNameParser) buildFHIRPath(tokens []string) string {
    // Example: [patient, id] → "id" (primary key)
    // Example: [phone, number] → "telecom[0].value"
    // Example: [observation, value, unit] → "valueQuantity.unit"
    
    // Implementation uses pattern matching and rule engine
    return buildPathFromTokens(p.resourceType, tokens)
}
```

---

## 7. Date/DateTime Format Handling

### Standard Date Mappings

```
SQL Format              → FHIR Format      → Pattern
YYYY-MM-DD             → YYYY-MM-DD       → Date
YYYY-MM-DD HH:MM:SS    → YYYY-MM-DDTHH:MM:SSZ → DateTime
DD/MM/YYYY             → YYYY-MM-DD       → Parse & convert
MM/DD/YYYY             → YYYY-MM-DD       → Parse & convert
```

### Auto-Detection Strategy

```go
func detectDateFormat(value string) string {
    formats := []string{
        "2006-01-02",                  // YYYY-MM-DD
        "2006-01-02T15:04:05Z",       // ISO 8601
        "2006-01-02T15:04:05-07:00",  // ISO with timezone
        "01/02/2006",                  // MM/DD/YYYY
        "02/01/2006",                  // DD/MM/YYYY
        "2006/01/02",                  // YYYY/MM/DD
    }
    
    for _, format := range formats {
        if _, err := time.Parse(format, value); err == nil {
            return format
        }
    }
    return "" // Unknown format
}

func convertToFHIRDateTime(value string, sourceFormat string, targetType string) string {
    t, _ := time.Parse(sourceFormat, value)
    
    if targetType == "date" {
        return t.Format("2006-01-02")
    } else {
        return t.Format("2006-01-02T15:04:05Z07:00")
    }
}
```

---

## 8. Code System and ValueSet Mapping

### Standard Mappings

```
Content Domain      → Code System URI
LOINC              → http://loinc.org
SNOMED CT          → http://snomed.info/sct
ICD-10-CM          → http://hl7.org/fhir/sid/icd-10-cm
ICD-9-CM           → http://hl7.org/fhir/sid/icd-9-cm
Gender             → http://hl7.org/fhir/administrative-gender
Marital Status     → http://terminology.hl7.org/CodeSystem/marital-status
Observation Status → http://hl7.org/fhir/observation-status
Condition Status   → http://terminology.hl7.org/CodeSystem/condition-clinical
Procedure Status   → http://hl7.org/fhir/event-status
```

### Column Name Pattern → Code System

```go
func inferCodeSystem(columnName string, code string) string {
    columnLower := strings.ToLower(columnName)
    
    // Direct inference
    if strings.Contains(columnLower, "loinc") {
        return "http://loinc.org"
    }
    if strings.Contains(columnLower, "snomed") || strings.Contains(columnLower, "sct") {
        return "http://snomed.info/sct"
    }
    if strings.Contains(columnLower, "icd-10") || strings.Contains(columnLower, "icd10") {
        return "http://hl7.org/fhir/sid/icd-10-cm"
    }
    if strings.Contains(columnLower, "icd-9") || strings.Contains(columnLower, "icd9") {
        return "http://hl7.org/fhir/sid/icd-9-cm"
    }
    
    // Pattern-based inference
    if strings.Contains(columnLower, "code") && strings.HasPrefix(code, "E") {
        return "http://hl7.org/fhir/sid/icd-10-cm" // ICD-10 starts with E
    }
    
    // Default/unknown
    return "http://example.com/" + columnName
}
```

---

## 9. Handling Multiple References and Arrays

### Pattern: Same Field from Multiple Columns

**SQL Table**:
```sql
CREATE TABLE patients (
    patient_id VARCHAR(20),
    first_name VARCHAR(50),
    middle_name VARCHAR(50),
    last_name VARCHAR(50),
    email_1 VARCHAR(100),
    email_2 VARCHAR(100),
    phone_1 VARCHAR(20),
    phone_2 VARCHAR(20)
);
```

**Auto-Derived Mapping**:
```json
{
    "first_name": "Patient.name[0].given[0]",
    "middle_name": "Patient.name[0].given[1]",
    "last_name": "Patient.name[0].family",
    "email_1": "Patient.telecom[0].value (system: email)",
    "email_2": "Patient.telecom[1].value (system: email)",
    "phone_1": "Patient.telecom[2].value (system: phone)",
    "phone_2": "Patient.telecom[3].value (system: phone)"
}
```

**Algorithm**:
```go
func consolidateArrayFields(resourceType string, columns []string) map[string][]string {
    // Group columns by base name and index
    grouped := make(map[string][]string)
    
    for _, col := range columns {
        baseField, index := extractBaseFieldAndIndex(col)
        grouped[baseField] = append(grouped[baseField], col)
    }
    
    // Determine if multiple columns should merge into single array
    result := make(map[string][]string)
    for baseField, cols := range grouped {
        if shouldMergeIntoArray(resourceType, baseField, cols) {
            result[baseField] = cols
        }
    }
    
    return result
}

func extractBaseFieldAndIndex(columnName string) (string, int) {
    // "email_1" → ("email", 1)
    // "phone_2" → ("phone", 2)
    // "first_name" → ("firstName", 0)
    
    parts := strings.Split(columnName, "_")
    lastPart := parts[len(parts)-1]
    
    if index, err := strconv.Atoi(lastPart); err == nil {
        return strings.Join(parts[:len(parts)-1], "_"), index
    }
    return columnName, 0
}
```

---

## 10. Configuration File Format

### Mapping Configuration (YAML)

```yaml
resourceType: Patient

# Auto-derived mappings (optional override)
columnMappings:
  # Key: SQL column name, Value: FHIR path
  patient_id: "id"
  first_name: "name[0].given[0]"
  last_name: "name[0].family"
  date_of_birth: "birthDate"
  gender: "gender"
  
# Date format specifications
dateFormats:
  - column: "date_of_birth"
    format: "YYYY-MM-DD"
  - column: "diagnosis_date"
    format: "DD/MM/YYYY"

# Code system mappings
codeSystems:
  - column: "gender"
    system: "http://hl7.org/fhir/administrative-gender"
  - column: "icd_code"
    system: "http://hl7.org/fhir/sid/icd-10-cm"

# Reference mappings
references:
  - column: "organization_id"
    resourceType: "Organization"
    fhirPath: "managingOrganization"
  - column: "practitioner_id"
    resourceType: "Practitioner"
    fhirPath: "generalPractitioner[0]"

# Array consolidation rules
arrayConsolidation:
  - baseField: "name"
    columns: ["first_name", "last_name", "middle_name"]
  - baseField: "telecom"
    columns: ["email_1", "email_2", "phone_1", "phone_2"]
```

---

## 11. Best Practices and Recommendations

### 1. Naming Convention Best Practices

✅ **DO**:
- Use snake_case for SQL columns
- Use clear, descriptive names: `date_of_birth` not `dob`
- Use consistent suffixes: `_id` for identifiers, `_code` for codes, `_date` for dates
- Prefix related columns: `contact_email`, `contact_phone`
- Use full resource names: `patient_id` not `pt_id`

❌ **DON'T**:
- Use single-letter abbreviations: `dob`, `gp`, `pt`
- Mix naming conventions within same table
- Use generic names: `value`, `code`, `date` without context
- Include FHIR-specific terminology in SQL: avoid `CodeableConcept_code`

### 2. Code System Standardization

✅ **DO**:
- Create a `code_system` column for every `*_code` column
- Use standardized URI patterns for code systems
- Document which code systems are used in your data

❌ **DON'T**:
- Assume code system from column name
- Use mixed code systems without tracking

### 3. Data Validation

✅ **DO**:
- Validate dates are in expected format
- Verify gender codes match allowed values: `male|female|other|unknown`
- Ensure status codes match FHIR allowed values
- Validate code systems are recognized

### 4. Handling Missing Data

| Missing Pattern | FHIR Handling |
|---|---|
| NULL/empty string | Omit field from JSON |
| "N/A", "unknown" | Include as text value |
| 0 or false | Include (may be meaningful) |
| Empty collection | Omit from JSON |

---

## 12. Summary: Quick Reference Guide

### Table Name to Resource Type
```
Pattern: {resource_type}_* → {ResourceType}
Example: patients → Patient
Example: observations → Observation
```

### Column Name to FHIR Path
```
Pattern: {field}_{modifier}_{component}
Example: phone_number → telecom[].value
Example: test_code → code.coding[].code
Example: test_display → code.text
```

### Automatic Type Detection
```
Suffix          → Type
_date          → date/dateTime
_code          → code
_id            → reference
_count         → integer
_flag          → boolean
_text          → string
```

### Special Handling
```
Resource        Key Fields to Recognize
Patient         first_name, last_name, date_of_birth, gender
Observation     code, value, unit, effective_date, status
Condition       code, clinical_status, onset_date, subject_id
Procedure       code, status, performed_date, performer_id
```

---

## 13. Implementation Roadmap

**Phase 1: Core Mapping Engine**
- [ ] Table name → Resource type inference
- [ ] Column name → FHIR field path extraction
- [ ] SQL type → FHIR type conversion
- [ ] Predefined mapping table (Patient, Observation, Condition)

**Phase 2: Complex Types**
- [ ] CodeableConcept assembly (code + display + system)
- [ ] Quantity assembly (value + unit + system + code)
- [ ] Reference creation and type inference
- [ ] Array/collection consolidation

**Phase 3: Configuration & Customization**
- [ ] YAML configuration support
- [ ] Override mappings
- [ ] Code system mappings
- [ ] Date format specifications

**Phase 4: Validation & Quality**
- [ ] FHIR resource validation
- [ ] Code system validation
- [ ] Error reporting and logging
- [ ] Data quality metrics

---

## References

- **FHIR Specification**: http://hl7.org/fhir/
- **FHIR Data Types**: http://hl7.org/fhir/datatypes.html
- **Resource Definitions**: http://hl7.org/fhir/resourcelist.html
- **Terminology Resources**: http://hl7.org/fhir/terminology-module.html
- **Common Code Systems**:
  - LOINC: http://loinc.org
  - SNOMED CT: http://snomed.info/sct
  - ICD-10: http://hl7.org/fhir/sid/icd-10
