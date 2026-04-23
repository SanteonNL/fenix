-- Luscii PatientTransformer → FHIR Patient
--
-- Tabellen aangemaakt door LusciiLoader:
--   luscii_patients              → hoofd-rij per patiënt (camelCase kolomnamen uit JSON-tags)
--   luscii_patients_identifiers  → één rij per identifier, FK: luscii_patient_id

-- ── Statement 1: Root Patient ──────────────────────────────────────────────
SELECT
    id                                                          AS resource_id,
    id                                                          AS id,
    ''                                                          AS parent_id,
    'Patient'                                                   AS fhir_path,
    CASE sex
        WHEN 'male'   THEN 'male'
        WHEN 'female' THEN 'female'
        ELSE               'unknown'
    END                                                         AS gender,
    dateOfBirth                                                 AS birthDate,
    CASE status WHEN 'active' THEN 'true' ELSE 'false' END      AS active,
    language
FROM luscii_patients;

-- ── Statement 2: Naam ─────────────────────────────────────────────────────
-- Eén naam per patiënt (official use), middleName optioneel als tweede given.
SELECT
    id                          AS resource_id,
    id || '_name'               AS id,
    id                          AS parent_id,
    'Patient.name'              AS fhir_path,
    'official'                  AS "use",
    lastName                    AS family,
    firstName                   AS given
FROM luscii_patients;

-- ── Statement 3: Identifier ───────────────────────────────────────────────
SELECT
    patient_id                              AS resource_id,
    patient_id || '_' || COALESCE(value, rowid) AS id,
    patient_id                              AS parent_id,
    'Patient.identifier'                    AS fhir_path,
    "use",
    system,
    value
FROM luscii_patients_identifiers;
