# Intelligent SQL-to-FHIR Mapping: Implementation Examples

## Example 1: Patient Conversion

### Input: SQL Table
```sql
CREATE TABLE patients (
    patient_id VARCHAR(20),
    first_name VARCHAR(50),
    last_name VARCHAR(50),
    date_of_birth DATE,
    gender VARCHAR(20),
    phone_number VARCHAR(20),
    email_address VARCHAR(100),
    active BOOLEAN,
    managing_organization_id VARCHAR(50)
);

-- Sample data
INSERT INTO patients VALUES 
  ('PT-001', 'John', 'Doe', '1990-01-15', 'male', '555-1234', 'john@example.com', true, 'ORG-001');
```

### Column Mapping Derivation

```go
// Table: patients → Patient resource
engine := NewIntelligentMappingEngine("Patient")

mappings := map[string]string{
    "patient_id":                "id",
    "first_name":                "name[0].given[0]",
    "last_name":                 "name[0].family",
    "date_of_birth":             "birthDate",
    "gender":                    "gender",
    "phone_number":              "telecom[0].value", // system: phone
    "email_address":             "telecom[1].value", // system: email
    "active":                    "active",
    "managing_organization_id":  "managingOrganization.reference"
}

for col, expectedPath := range mappings {
    derivedPath := engine.AutoDeriveMapping(col).FHIRPath
    assert(derivedPath == expectedPath, col)
}
```

### Output: FHIR Resource
```json
{
    "resourceType": "Patient",
    "id": "PT-001",
    "name": [
        {
            "given": ["John"],
            "family": "Doe",
            "text": "John Doe"
        }
    ],
    "telecom": [
        {
            "system": "phone",
            "value": "555-1234"
        },
        {
            "system": "email",
            "value": "john@example.com"
        }
    ],
    "birthDate": "1990-01-15",
    "gender": "male",
    "active": true,
    "managingOrganization": {
        "reference": "Organization/ORG-001"
    }
}
```

---

## Example 2: Laboratory Observation Conversion

### Input: SQL Table
```sql
CREATE TABLE lab_results (
    result_id VARCHAR(20),
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
    reference_high DECIMAL(10,2),
    performer_id VARCHAR(50)
);

-- Sample data
INSERT INTO lab_results VALUES 
  ('OBS-001', 'PT-001', '2345-7', 'Glucose', 'http://loinc.org', 95.5, 'mg/dL', 
   'http://unitsofmeasure.org', '2024-01-15 09:30:00', 'final', 70, 100, 'PRAC-001');
```

### Column Mapping Derivation

```go
engine := NewIntelligentMappingEngine("Observation")

mappings := map[string]string{
    "result_id":           "id",
    "patient_id":          "subject.reference",
    "test_code":           "code.coding[0].code",
    "test_name":           "code.text",
    "test_system":         "code.coding[0].system",
    "result_value":        "valueQuantity.value",
    "result_unit":         "valueQuantity.unit",
    "unit_system":         "valueQuantity.system",
    "test_date":           "effectiveDateTime",
    "status":              "status",
    "reference_low":       "referenceRange[0].low.value",
    "reference_high":      "referenceRange[0].high.value",
    "performer_id":        "performer[0].reference"
}

for col, expectedPath := range mappings {
    derivedPath := engine.AutoDeriveMapping(col).FHIRPath
    assert(derivedPath == expectedPath, col)
}
```

### Output: FHIR Resource
```json
{
    "resourceType": "Observation",
    "id": "OBS-001",
    "status": "final",
    "code": {
        "coding": [
            {
                "system": "http://loinc.org",
                "code": "2345-7",
                "display": "Glucose [mg/dL]"
            }
        ],
        "text": "Glucose [mg/dL]"
    },
    "subject": {
        "reference": "Patient/PT-001"
    },
    "effectiveDateTime": "2024-01-15T09:30:00Z",
    "valueQuantity": {
        "value": "95.5",
        "unit": "mg/dL",
        "system": "http://unitsofmeasure.org",
        "code": "mg/dL"
    },
    "referenceRange": [
        {
            "low": {
                "value": "70"
            },
            "high": {
                "value": "100"
            }
        }
    ],
    "performer": [
        {
            "reference": "Practitioner/PRAC-001"
        }
    ]
}
```

---

## Example 3: Diagnosis/Condition Conversion

### Input: SQL Table
```sql
CREATE TABLE diagnoses (
    condition_id VARCHAR(20),
    patient_id VARCHAR(20),
    icd_code VARCHAR(20),
    diagnosis_name VARCHAR(200),
    diagnosis_system VARCHAR(100),
    clinical_status VARCHAR(50),
    verification_status VARCHAR(50),
    severity VARCHAR(50),
    diagnosis_date DATE,
    recorded_date DATE,
    recorded_by VARCHAR(50)
);

-- Sample data
INSERT INTO diagnoses VALUES 
  ('COND-001', 'PT-001', 'E11.9', 'Type 2 diabetes mellitus', 
   'http://hl7.org/fhir/sid/icd-10-cm', 'active', 'confirmed', 'moderate',
   '2015-03-20', '2024-01-15', 'PRAC-001');
```

### Column Mapping Derivation

```go
engine := NewIntelligentMappingEngine("Condition")

mappings := map[string]string{
    "condition_id":         "id",
    "patient_id":           "subject.reference",
    "icd_code":             "code.coding[0].code",
    "diagnosis_name":       "code.text",
    "diagnosis_system":     "code.coding[0].system",
    "clinical_status":      "clinicalStatus.coding[0].code",
    "verification_status":  "verificationStatus.coding[0].code",
    "severity":             "severity.text",
    "diagnosis_date":       "onsetDateTime",
    "recorded_date":        "recordedDate",
    "recorded_by":          "recorder.reference"
}

for col, expectedPath := range mappings {
    derivedPath := engine.AutoDeriveMapping(col).FHIRPath
    assert(derivedPath == expectedPath, col)
}
```

### Output: FHIR Resource
```json
{
    "resourceType": "Condition",
    "id": "COND-001",
    "code": {
        "coding": [
            {
                "system": "http://hl7.org/fhir/sid/icd-10-cm",
                "code": "E11.9",
                "display": "Type 2 diabetes mellitus without complications"
            }
        ],
        "text": "Type 2 diabetes mellitus"
    },
    "subject": {
        "reference": "Patient/PT-001"
    },
    "clinicalStatus": {
        "coding": [
            {
                "system": "http://terminology.hl7.org/CodeSystem/condition-clinical",
                "code": "active"
            }
        ],
        "text": "active"
    },
    "verificationStatus": {
        "coding": [
            {
                "system": "http://terminology.hl7.org/CodeSystem/condition-verification",
                "code": "confirmed"
            }
        ],
        "text": "confirmed"
    },
    "severity": {
        "text": "moderate"
    },
    "onsetDateTime": "2015-03-20",
    "recordedDate": "2024-01-15",
    "recorder": {
        "reference": "Practitioner/PRAC-001"
    }
}
```

---

## Example 4: Complex Arrays - Multiple Names and Contact Methods

### Input: SQL Table
```sql
CREATE TABLE patients_extended (
    patient_id VARCHAR(20),
    first_name VARCHAR(50),
    middle_name VARCHAR(50),
    last_name VARCHAR(50),
    name_prefix VARCHAR(10),
    email_1 VARCHAR(100),
    email_2 VARCHAR(100),
    phone_1 VARCHAR(20),
    phone_2 VARCHAR(20),
    fax_number VARCHAR(20)
);

-- Sample data
INSERT INTO patients_extended VALUES 
  ('PT-001', 'John', 'Michael', 'Doe', 'Dr.', 
   'john.doe@example.com', 'j.doe@work.com', '555-1234', '555-5678', '555-9999');
```

### Column Mapping Derivation with Array Consolidation

```go
engine := NewIntelligentMappingEngine("Patient")

// Phase 1: Individual column mappings
mappings := map[string]string{
    "first_name":  "name[0].given[0]",
    "middle_name": "name[0].given[1]",
    "last_name":   "name[0].family",
    "name_prefix": "name[0].prefix[0]",
    "email_1":     "telecom[0].value",    // system: email
    "email_2":     "telecom[1].value",    // system: email
    "phone_1":     "telecom[2].value",    // system: phone
    "phone_2":     "telecom[3].value",    // system: phone
    "fax_number":  "telecom[4].value",    // system: fax
}

// Phase 2: Array consolidation (telecom[0-4] are consolidated)
// The converter understands that multiple telecom entries should be arrays
```

### Output: FHIR Resource
```json
{
    "resourceType": "Patient",
    "id": "PT-001",
    "name": [
        {
            "prefix": ["Dr."],
            "given": ["John", "Michael"],
            "family": "Doe",
            "text": "Dr. John Michael Doe"
        }
    ],
    "telecom": [
        {"system": "email", "value": "john.doe@example.com"},
        {"system": "email", "value": "j.doe@work.com"},
        {"system": "phone", "value": "555-1234"},
        {"system": "phone", "value": "555-5678"},
        {"system": "fax", "value": "555-9999"}
    ]
}
```

---

## Example 5: CodeableConcept Assembly

### Input: SQL Table (Separated Code Components)
```sql
CREATE TABLE observations_coded (
    obs_id VARCHAR(20),
    patient_id VARCHAR(20),
    observation_code VARCHAR(20),
    observation_code_display VARCHAR(100),
    observation_code_system VARCHAR(100),
    observation_value_code VARCHAR(20),
    observation_value_display VARCHAR(100),
    observation_value_system VARCHAR(100),
    observation_date TIMESTAMP
);

-- Sample data
INSERT INTO observations_coded VALUES 
  ('OBS-002', 'PT-001', 'LAB-001', 'Complete Blood Count', 'http://example.com/lab-codes',
   'NOR', 'Normal', 'http://example.com/result-codes', '2024-01-15 10:00:00');
```

### Column Mapping with CodeableConcept Assembly

```go
engine := NewIntelligentMappingEngine("Observation")

// Individual column mappings
mappings := map[string]string{
    "obs_id":                        "id",
    "patient_id":                    "subject.reference",
    "observation_code":              "code.coding[0].code",
    "observation_code_display":      "code.text",
    "observation_code_system":       "code.coding[0].system",
    "observation_value_code":        "valueCodeableConcept.coding[0].code",
    "observation_value_display":     "valueCodeableConcept.text",
    "observation_value_system":      "valueCodeableConcept.coding[0].system",
    "observation_date":              "effectiveDateTime"
}

// Intelligent assembly:
// observation_code* → CodeableConcept {
//     coding[0].code: observation_code,
//     coding[0].display: observation_code_display,
//     coding[0].system: observation_code_system,
//     text: observation_code_display
// }
```

### Output: FHIR Resource
```json
{
    "resourceType": "Observation",
    "id": "OBS-002",
    "code": {
        "coding": [
            {
                "system": "http://example.com/lab-codes",
                "code": "LAB-001",
                "display": "Complete Blood Count"
            }
        ],
        "text": "Complete Blood Count"
    },
    "subject": {
        "reference": "Patient/PT-001"
    },
    "valueCodeableConcept": {
        "coding": [
            {
                "system": "http://example.com/result-codes",
                "code": "NOR",
                "display": "Normal"
            }
        ],
        "text": "Normal"
    },
    "effectiveDateTime": "2024-01-15T10:00:00Z"
}
```

---

## Example 6: Data Type Inference

### Input: SQL Types and Values
```sql
CREATE TABLE test_types (
    id VARCHAR(20),
    age_years INTEGER,
    weight_kg DECIMAL(5,2),
    is_active BOOLEAN,
    created_date DATE,
    last_modified TIMESTAMP,
    description TEXT
);
```

### Type Detection and Inference

```go
engine := NewIntelligentMappingEngine("Patient")

testCases := map[string]string{
    "id":              "string",
    "age_years":       "integer",
    "weight_kg":       "decimal",
    "is_active":       "boolean",
    "created_date":    "date",
    "last_modified":   "dateTime",
    "description":     "string",
}

// Derivation rules:
// 1. Column name pattern: _date → "date", _flag/_active → "boolean"
// 2. SQL type: DATE → "date", TIMESTAMP → "dateTime", INTEGER → "integer"
// 3. Combination: is_active (prefix "is_") → "boolean"
```

---

## Example 7: Reference Field Resolution

### Input: Multiple ID Columns
```sql
CREATE TABLE observations_with_refs (
    obs_id VARCHAR(20),
    patient_id VARCHAR(20),
    performer_id VARCHAR(50),
    encounter_id VARCHAR(20),
    specimen_id VARCHAR(20),
    device_id VARCHAR(20)
);
```

### Reference Type Inference

```go
engine := NewIntelligentMappingEngine("Observation")

// Entity type inference from column name
references := map[string]string{
    "patient_id":    "subject.reference",         // → Patient resource
    "performer_id":  "performer[0].reference",    // → Practitioner resource
    "encounter_id":  "encounter.reference",       // → Encounter resource
    "specimen_id":   "specimen.reference",        // → Specimen resource
    "device_id":     "device.reference",          // → Device resource
}

// Generated reference strings
// "Patient/PT-001"
// "Practitioner/PRAC-001"
// "Encounter/ENC-001"
// "Specimen/SPEC-001"
// "Device/DEV-001"
```

---

## Example 8: Date Format Auto-Detection

### Input: Various Date Formats
```sql
CREATE TABLE mixed_dates (
    id VARCHAR(20),
    date_format_iso DATE,          -- 2024-01-15
    date_format_us VARCHAR(20),    -- 01/15/2024
    date_format_eu VARCHAR(20),    -- 15/01/2024
    datetime_iso TIMESTAMP,        -- 2024-01-15T09:30:00Z
    datetime_unix INTEGER          -- 1705314600
);
```

### Format Detection and Conversion

```go
import "time"

testCases := []struct {
    Value       string
    DetectedFmt string
    FHIROutput  string
}{
    {"2024-01-15", "2006-01-02", "2024-01-15"},
    {"01/15/2024", "01/02/2006", "2024-01-15"},
    {"15/01/2024", "02/01/2006", "2024-01-15"},
    {"2024-01-15T09:30:00Z", time.RFC3339, "2024-01-15T09:30:00Z"},
}

for _, tc := range testCases {
    detected, _ := DetectDateFormat(tc.Value)
    assert(detected == tc.DetectedFmt)
    
    output, _ := ConvertToFHIRDate(tc.Value, detected)
    assert(output == tc.FHIROutput)
}
```

---

## Example 9: Code System Inference

### Input: Code Columns with Different Systems
```sql
CREATE TABLE coded_fields (
    id VARCHAR(20),
    gender_code VARCHAR(20),           -- Should infer FHIR gender
    loinc_code VARCHAR(20),            -- Should infer LOINC
    snomed_code VARCHAR(20),           -- Should infer SNOMED
    icd10_code VARCHAR(20),            -- Should infer ICD-10-CM
    observation_status_code VARCHAR(20) -- Should infer observation-status
);
```

### Code System Inference

```go
testCases := map[string]string{
    "gender_code":               "http://hl7.org/fhir/administrative-gender",
    "loinc_code":                "http://loinc.org",
    "snomed_code":               "http://snomed.info/sct",
    "icd10_code":                "http://hl7.org/fhir/sid/icd-10-cm",
    "observation_status_code":   "http://hl7.org/fhir/observation-status",
}

for col, expectedSystem := range testCases {
    inferredSystem := InferCodeSystem(col, "")
    assert(inferredSystem == expectedSystem, col)
}
```

---

## Integration with Converter

### Updated Converter Usage

```go
import "github.com/SanteonNL/fenix/cmd/csv2fhir/converter"

func ConvertTableToFHIRWithIntelligentMapping(
    tableName string,
    resourceType string,
    rows []map[string]interface{},
) []interface{} {
    
    // Create intelligent mapping engine
    engine := converter.NewIntelligentMappingEngine(resourceType)
    
    // Infer resource type if not specified
    if resourceType == "" {
        resourceType = converter.InferResourceTypeFromTableName(tableName)
    }
    
    var resources []interface{}
    
    for _, row := range rows {
        resource := make(map[string]interface{})
        
        // Process each column
        for column, value := range row {
            // Auto-derive mapping
            mapping := engine.AutoDeriveMapping(column)
            
            // Transform value based on data type
            transformedValue := transformValue(value, mapping.DataType)
            
            // Set in resource
            setNestedField(resource, mapping.FHIRPath, transformedValue)
        }
        
        resources = append(resources, resource)
    }
    
    return resources
}

func transformValue(value interface{}, dataType string) interface{} {
    switch dataType {
    case "date":
        dateStr, _ := converter.ConvertToFHIRDate(value, "")
        return dateStr
    case "code":
        return converter.MapValueToCodeableConcept(value.(string), "", "")
    case "reference":
        return converter.MapValueToReference("", value.(string))
    case "boolean":
        // Handle boolean conversion
        return value
    case "integer", "decimal":
        // Handle numeric conversion
        return value
    default:
        return value
    }
}
```

---

## Test Cases Summary

| Resource | Column Pattern | Expected FHIR Field | Test Case |
|---|---|---|---|
| Patient | first_name | name[0].given[0] | ✓ |
| Patient | date_of_birth | birthDate | ✓ |
| Patient | managing_org_id | managingOrganization.reference | ✓ |
| Observation | test_code | code.coding[0].code | ✓ |
| Observation | result_value | valueQuantity.value | ✓ |
| Observation | result_unit | valueQuantity.unit | ✓ |
| Condition | icd_code | code.coding[0].code | ✓ |
| Condition | clinical_status | clinicalStatus.coding[0].code | ✓ |
| Any | *_date | [field]DateTime | ✓ |
| Any | *_code | [field].coding[0].code | ✓ |
| Any | *_id (non-patient) | [field].reference | ✓ |
| Any | is_* | boolean | ✓ |

---

## Benefits of Intelligent Mapping

1. **Zero Configuration**: Works with standard naming conventions
2. **Extensible**: Add new patterns and resources easily
3. **Intelligent**: Infers complex types (CodeableConcept, Quantity)
4. **Standardized**: Follows FHIR and SQL naming best practices
5. **Maintainable**: Clear, rule-based logic
6. **Validated**: Ensures FHIR compliance
