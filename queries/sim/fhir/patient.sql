-- SIM → FHIR Patient
--
-- Table loaded from test/data/sim/Patient.csv (prefix = "sim"):
--   sim_patient
--     Identificatienummer, GeslachtCode, GeslachtOmschrijving,
--     Land, Geboortedatum, DatumOverlijden, DatumCheckStatusOverlijden

-- ── Statement 1: Root Patient ──────────────────────────────────────────────
SELECT
    Identificatienummer     AS resource_id,
    Identificatienummer     AS id,
    ''                      AS parent_id,
    'Patient'               AS fhir_path,
    CASE GeslachtCode
        WHEN 'M' THEN 'male'
        WHEN 'F' THEN 'female'
        ELSE          'unknown'
    END                     AS gender,
    Geboortedatum           AS birthDate,
    CASE WHEN DatumOverlijden IS NULL OR DatumOverlijden = ''
        THEN 'false'
    END                     AS deceasedBoolean,
    CASE WHEN DatumOverlijden IS NOT NULL AND DatumOverlijden != ''
        THEN DatumOverlijden
    END                     AS deceasedDateTime
FROM sim_patient;

-- ── Statement 2: Identifier (BSN) ─────────────────────────────────────────
SELECT
    Identificatienummer                             AS resource_id,
    Identificatienummer || '_bsn'                   AS id,
    Identificatienummer                             AS parent_id,
    'Patient.identifier'                            AS fhir_path,
    'official'                                      AS "use",
    'http://fhir.nl/fhir/NamingSystem/bsn'          AS "system",
    Identificatienummer                             AS "value"
FROM sim_patient;
