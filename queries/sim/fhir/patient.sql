-- SIM → FHIR Patient
--
-- Table loaded from test/data/sim/Patient.csv (prefix = "sim"):
--   sim_patient
--     Identificatienummer, GeslachtCode, GeslachtOmschrijving,
--     Land, Geboortedatum, DatumOverlijden, DatumCheckStatusOverlijden

-- ── Statement 1: Root Patient ──────────────────────────────────────────────
-- gender is passed through as the raw GeslachtCode, mapped to the FHIR
-- AdministrativeGender codes at conversion time by the ConceptMap in
-- terminology/conceptmaps/fhir/sim-administrative-gender.json (NOTE: keep
-- comments like this one on their own "--" line, never trailing on a SQL
-- line — SplitStatements naively splits the whole file on every statement
-- separator character, so one inside a trailing comment corrupts the query).
SELECT
    Identificatienummer     AS resource_id,
    Identificatienummer     AS id,
    ''                      AS parent_id,
    'Patient'               AS fhir_path,
    GeslachtCode            AS gender,
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
