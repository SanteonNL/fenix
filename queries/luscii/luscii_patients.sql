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
    luscii_patient_id                                           AS resource_id,
    luscii_patient_id || '_' || COALESCE(value, CAST(rowid AS TEXT)) AS id,
    luscii_patient_id                                           AS parent_id,
    'Patient.identifier'                                        AS fhir_path,
    "use",
    system,
    value
FROM luscii_patients_identifiers;

-- ── Statement 4: HIX Patient Number (via BSN mapping) ────────────────────
-- Links Luscii patients to HIX via the BSN mapping table
SELECT
    lpi.luscii_patient_id                                       AS resource_id,
    lpi.luscii_patient_id || '_hix_link'                        AS id,
    lpi.luscii_patient_id                                       AS parent_id,
    'Patient.identifier'                                        AS fhir_path,
    'secondary'                                                 AS "use",
    'http://example.com/hix-patient-number'                     AS system,
    hm.hix_patient_number                                       AS value
FROM luscii_patients_identifiers lpi
LEFT JOIN hix_bsn_mapping hm ON lpi.value = hm.bsn
WHERE lpi.system = 'http://fhir.nl/fhir/NamingSystem/bsn'
  AND hm.hix_patient_number IS NOT NULL;
