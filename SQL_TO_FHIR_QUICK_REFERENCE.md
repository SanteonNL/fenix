# SQL to FHIR: Quick Reference Card

## 1. Resource Type Detection from Table Names

```
Table Name Pattern          → FHIR Resource
patients, patient_*         → Patient
observations, observation_* → Observation
conditions, condition_*     → Condition
procedures, procedure_*     → Procedure
organizations, org_*        → Organization
encounters, encounter_*     → Encounter
medications, medication_*   → Medication

Prefix: pt_, obs_, cond_, proc_, enc_ → [Resource type]
```

---

## 2. Column Name Patterns (ALL Resources)

### Primary Key
```
Column Pattern      → FHIR Field
id                  → id
[resource]_id       → id
```

### Date/Time
```
Column Pattern      → FHIR Type
*_date              → date (YYYY-MM-DD)
*_datetime          → dateTime (YYYY-MM-DDTHH:mm:ssZ)
*_time              → time
created_*           → dateTime
modified_*          → dateTime
```

### Codes (CodeableConcept)
```
Column Pattern      → FHIR Structure
code                → code.coding[0].code
code_display        → code.text
code_system         → code.coding[0].system
[field]_code        → [field].coding[0].code
[field]_display     → [field].text
[field]_system      → [field].coding[0].system
```

### Quantities
```
Column Pattern      → FHIR Structure
value               → value (numeric)
value_unit          → valueQuantity.unit
unit                → valueQuantity.unit
[field]_value       → [field].value
[field]_unit        → [field].unit
[field]_system      → [field].system
```

### References
```
Column Pattern      → FHIR Resource Inferred
patient_id          → Patient
subject_id          → Patient
performer_id        → Practitioner
recorder_id         → Practitioner
organization_id     → Organization
encounter_id        → Encounter
location_id         → Location
specimen_id         → Specimen
device_id           → Device
```

### Boolean/Flags
```
Column Pattern      → FHIR Type
is_*                → boolean
*_flag              → boolean
active              → boolean
enabled             → boolean
```

### Collections/Arrays
```
Column Pattern      → Array Handling
*_1, *_2, *_3       → array[0], array[1], array[2]
multiple_*          → array (consolidated)
email_1, phone_1    → telecom[0], telecom[1]
```

---

## 3. Patient Resource Mappings

```
SQL Column              → FHIR Field
id, patient_id          → Patient.id
first_name, given_name  → Patient.name[0].given[0]
middle_name             → Patient.name[0].given[1]
last_name, family_name  → Patient.name[0].family
full_name, name         → Patient.name[0].text
gender, sex             → Patient.gender
date_of_birth, birthdate → Patient.birthDate
date_of_death           → Patient.deceasedDateTime
active, is_active       → Patient.active
email, email_address    → Patient.telecom[].value (system: email)
phone, phone_number     → Patient.telecom[].value (system: phone)
fax, fax_number         → Patient.telecom[].value (system: fax)
address, street         → Patient.address[0].line[0]
city                    → Patient.address[0].city
state, province         → Patient.address[0].state
postal_code, zip_code   → Patient.address[0].postalCode
country                 → Patient.address[0].country
marital_status          → Patient.maritalStatus.text
language, preferred_*   → Patient.communication[0].language.text
managing_org_id         → Patient.managingOrganization.reference
gp_id, general_pract_id → Patient.generalPractitioner[0].reference
```

---

## 4. Observation Resource Mappings

```
SQL Column              → FHIR Field
id, observation_id      → Observation.id
status                  → Observation.status
code, test_code         → Observation.code.coding[0].code
code_display, test_name → Observation.code.text
code_system, test_system → Observation.code.coding[0].system
value, result           → Observation.valueQuantity.value
value_unit, unit        → Observation.valueQuantity.unit
unit_system             → Observation.valueQuantity.system
unit_code               → Observation.valueQuantity.code
value_code, result_code → Observation.valueCodeableConcept.coding[0].code
value_display           → Observation.valueCodeableConcept.text
value_string            → Observation.valueString
value_boolean           → Observation.valueBoolean
effective_date, test_date → Observation.effectiveDateTime
issued_date, result_date  → Observation.issued
subject_id, patient_id  → Observation.subject.reference
performer_id            → Observation.performer[0].reference
reference_range_low     → Observation.referenceRange[0].low.value
reference_range_high    → Observation.referenceRange[0].high.value
normal_low, normal_high → Observation.referenceRange[0]
category                → Observation.category[0].text
interpretation, flag    → Observation.interpretation[0].text
note, comment           → Observation.note[0].text
```

---

## 5. Condition Resource Mappings

```
SQL Column              → FHIR Field
id, condition_id        → Condition.id
status, clinical_status → Condition.clinicalStatus.coding[0].code
verification_status     → Condition.verificationStatus.coding[0].code
code, diagnosis_code    → Condition.code.coding[0].code
code_display, diagnosis_name → Condition.code.text
code_system, diagnosis_system → Condition.code.coding[0].system
category                → Condition.category[0].text
severity                → Condition.severity.text
body_site, affected_site → Condition.bodySite[0].text
subject_id, patient_id  → Condition.subject.reference
encounter_id            → Condition.encounter.reference
onset_date, start_date  → Condition.onsetDateTime
abatement_date, end_date → Condition.abatementDateTime
recorded_date           → Condition.recordedDate
recorder_id, recorded_by → Condition.recorder.reference
asserter_id, asserted_by → Condition.asserter.reference
note, comment           → Condition.note[0].text
```

---

## 6. Procedure Resource Mappings

```
SQL Column              → FHIR Field
id, procedure_id        → Procedure.id
status                  → Procedure.status
code, procedure_code    → Procedure.code.coding[0].code
code_display, proc_name → Procedure.code.text
category                → Procedure.category.text
subject_id, patient_id  → Procedure.subject.reference
encounter_id            → Procedure.encounter.reference
performed_date          → Procedure.performedDateTime
performer_id, surgeon_id → Procedure.performer[0].actor.reference
location_id             → Procedure.location.reference
body_site, site         → Procedure.bodySite[0].text
outcome, result         → Procedure.outcome.text
note, report            → Procedure.note[0].text
```

---

## 7. Data Type Inference Rules

```
Column Name Pattern         → SQL Type         → FHIR Type
*_date                      → DATE              → date
*_datetime, *_timestamp     → DATETIME/TIMESTAMP → dateTime
*_code                      → VARCHAR           → code
*_id                        → VARCHAR           → reference
is_*, *_flag, active        → BOOLEAN           → boolean
*_count, *_number           → INTEGER           → integer
*_percent, *_amount         → DECIMAL/FLOAT     → decimal
*_text, *_name, *_display   → VARCHAR/TEXT      → string
(default)                   → VARCHAR           → string
```

---

## 8. Special Type Handling

### CodeableConcept (From Multiple Columns)
```
SQL                                 FHIR
code        +                       CodeableConcept {
code_display    +       →           coding[0].code
code_system     +                   coding[0].display
                                    coding[0].system
                                    text
}
```

### Quantity (Value + Unit)
```
SQL                                 FHIR
value       +                       Quantity {
unit        +       →               value
system                              unit
code                                system
                                    code
}
```

### Reference (Foreign Keys)
```
SQL                                 FHIR
performer_id    →                   Performer (Reference)
    value: PT-001                   reference: "Practitioner/PT-001"
                                    type: "Practitioner"
```

### Array Consolidation
```
SQL                                 FHIR
email_1        →                    telecom[0]
email_2        →                    telecom[1]
phone_1        →                    telecom[2]
phone_2        →                    telecom[3]
```

---

## 9. Code System Mappings

```
Column Contains         → Code System
loinc                   → http://loinc.org
snomed, sct             → http://snomed.info/sct
icd-10, icd10           → http://hl7.org/fhir/sid/icd-10-cm
icd-9, icd9             → http://hl7.org/fhir/sid/icd-9-cm
gender                  → http://hl7.org/fhir/administrative-gender
status (observation)    → http://hl7.org/fhir/observation-status
status (condition)      → http://terminology.hl7.org/CodeSystem/condition-clinical
status (procedure)      → http://hl7.org/fhir/event-status
marital                 → http://terminology.hl7.org/CodeSystem/marital-status
```

---

## 10. Common SQL Type Mappings

```
SQL Type                    → FHIR Type
VARCHAR, TEXT, CHAR         → string
INT, INTEGER, BIGINT        → integer
FLOAT, REAL, DOUBLE         → decimal
DECIMAL, NUMERIC            → decimal
BOOLEAN, BOOL               → boolean
DATE                        → date (YYYY-MM-DD)
DATETIME, TIMESTAMP         → dateTime (ISO 8601)
DATETIME2, TIMESTAMP WITH TZ → dateTime (ISO 8601)
JSON, JSONB                 → json (complex)
```

---

## 11. Date Format Auto-Detection

```
Format              Pattern
YYYY-MM-DD          2024-01-15
YYYY-MM-DDTHH:MM:SS 2024-01-15T09:30:00
ISO 8601            2024-01-15T09:30:00Z
ISO with TZ         2024-01-15T09:30:00-05:00
DD/MM/YYYY          15/01/2024
MM/DD/YYYY          01/15/2024
YYYY/MM/DD          2024/01/15
```

---

## 12. Contact Point System Detection

```
Column Contains     → System Value
email, mail         → email
phone, tel          → phone
fax                 → fax
url, website        → url
sms, text           → sms
other               → other
pager               → pager
```

---

## 13. Gender Code Mapping

```
Input Value → FHIR Code
male        → male
M           → male
m           → male
female      → female
F           → female
f           → female
other       → other
unknown     → unknown
U           → unknown
```

---

## 14. Observation Status Values

```
Valid FHIR Values:
registered, preliminary, final, amended, corrected, appended, cancelled, 
entered-in-error, unknown
```

---

## 15. Condition Clinical Status

```
Valid FHIR Values:
active, recurrence, relapse, inactive, remission, resolved
```

---

## 16. Procedure Status

```
Valid FHIR Values:
preparation, in-progress, suspended, aborted, completed, entered-in-error, unknown
```

---

## 17. Boolean Value Conversion

```
Truthy Values:          → true
true, 1, yes, y, on     → true

Falsy Values:           → false
false, 0, no, n, off    → false

NULL/Empty             → Omit field
"", null, undefined
```

---

## 18. Quick Checklist: Before Conversion

- [ ] Table name suggests resource type
- [ ] Columns use snake_case naming
- [ ] Primary key is named "id" or "[resource]_id"
- [ ] Date columns end with "_date" or "_datetime"
- [ ] Code columns end with "_code"
- [ ] Foreign keys end with "_id"
- [ ] Boolean columns start with "is_" or end with "_flag"
- [ ] Complex fields have matching _display and _system columns
- [ ] Collection columns use numeric suffixes (_1, _2, etc.)
- [ ] Code systems are documented or inferrable from column names

---

## 19. Common Mistakes to Avoid

❌ **DON'T**:
```
❌ Use abbreviations: pt_id, obs, dob, gp
❌ Mix naming conventions: PatientID, patient-id, PATIENT_ID
❌ Create ambiguous names: code, value, name (without context)
❌ Forget code systems: Only have code, no code_system column
❌ Use cryptic suffixes: _val, _cd, _desc
❌ Single letter columns: a, b, x, y
```

✅ **DO**:
```
✅ Use full names: patient_id, observation_id
✅ Be consistent: snake_case throughout
✅ Be specific: observation_code, patient_name
✅ Include code system: Include code_system column
✅ Use descriptive suffixes: _code, _date, _value
✅ Name all columns: No single letters
```

---

## 20. Example: Complete Table Mapping

### SQL Table:
```sql
CREATE TABLE observations (
    observation_id VARCHAR(20),
    patient_id VARCHAR(20),
    test_code VARCHAR(20),
    test_display VARCHAR(100),
    test_system VARCHAR(100),
    result_value DECIMAL(10,2),
    result_unit VARCHAR(20),
    unit_system VARCHAR(100),
    effective_date TIMESTAMP,
    status VARCHAR(20),
    performer_id VARCHAR(50)
);
```

### Auto-Derived Mappings:
```json
{
    "observation_id": "id",
    "patient_id": "subject.reference",
    "test_code": "code.coding[0].code",
    "test_display": "code.text",
    "test_system": "code.coding[0].system",
    "result_value": "valueQuantity.value",
    "result_unit": "valueQuantity.unit",
    "unit_system": "valueQuantity.system",
    "effective_date": "effectiveDateTime",
    "status": "status",
    "performer_id": "performer[0].reference"
}
```

### Automatic Processing:
1. Table name "observations" → Resource type "Observation"
2. Each column auto-mapped using intelligent engine
3. References (patient_id, performer_id) automatically resolve to resource types
4. Code components (code, display, system) automatically assembled into CodeableConcept
5. Quantity fields (value, unit, system) automatically assembled into Quantity
6. Date automatically converted to FHIR format

✅ **Zero configuration needed!**
