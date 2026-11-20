-- SIM Verrichting → FHIR Procedure (ProcedureSan)
--
-- Table loaded from test/data/sim/Verrichting.csv (prefix "sim"):
--   sim_verrichting
--     VerrichtingID, Identificatienummer,
--     VerrichtingTypeCodeNZaSysteem, VerrichtingTypeCodeNZa, VerrichtingTypeOmschrijvingNZa,
--     VerrichtingStartDatum, SubtrajectNr
--
-- The SIM has no status: a registered verrichting is taken as completed.
-- Template var .subject (FHIR patient param, pushdown in config/queries/sources/sim/sim.yaml).

-- ── Statement 1: Root Procedure ───────────────────────────────────────────
SELECT
    VerrichtingID                           AS resource_id,
    VerrichtingID                           AS id,
    ''                                      AS parent_id,
    'Procedure'                             AS fhir_path,
    'completed'                             AS status,
    'Patient/' || Identificatienummer       AS "subject.reference",
    NULLIF(VerrichtingStartDatum, '')       AS performedDateTime
FROM sim_verrichting
WHERE 1=1
{{- if .subject}} AND Identificatienummer = '{{.subject}}'{{end}};

-- ── Statement 2: identifier ───────────────────────────────────────────────
SELECT
    VerrichtingID                           AS resource_id,
    VerrichtingID || '_identifier'          AS id,
    VerrichtingID                           AS parent_id,
    'Procedure.identifier'                  AS fhir_path,
    'urn:santeon:sim:verrichting'           AS "system",
    VerrichtingID                           AS "value"
FROM sim_verrichting
WHERE 1=1
{{- if .subject}} AND Identificatienummer = '{{.subject}}'{{end}};

-- ── Statement 3: code.coding (VerrichtingType, NZa zorgactiviteit) ────────
SELECT
    VerrichtingID                           AS resource_id,
    VerrichtingID || '_code'                AS id,
    VerrichtingID                           AS parent_id,
    'Procedure.code.coding'                 AS fhir_path,
    VerrichtingTypeCodeNZaSysteem           AS "system",
    VerrichtingTypeCodeNZa                  AS code,
    VerrichtingTypeOmschrijvingNZa          AS display
FROM sim_verrichting
WHERE VerrichtingTypeCodeNZa IS NOT NULL AND VerrichtingTypeCodeNZa != ''
{{- if .subject}} AND Identificatienummer = '{{.subject}}'{{end}};
