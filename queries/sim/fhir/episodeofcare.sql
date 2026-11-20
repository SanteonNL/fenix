-- SIM DBC → FHIR EpisodeOfCare (EpisodeOfCareSan)
--
-- Table loaded from test/data/sim/DBC.csv (prefix "sim"):
--   sim_dbc
--     SubtrajectNr, Identificatienummer,
--     ZorgTypeCodeSysteem, ZorgTypeCode, ZorgTypeOmschrijving,
--     DBCOpeningsDatum, DBCSluitingsDatum, DBCgeldig,
--     SpecialismeDiagnoseSysteem, SpecialismeDiagnoseCode, SpecialismeDiagnoseOmschrijving
--
-- status follows the SanEpisodeOfCareStatus rule: not valid -> entered-in-error,
-- valid and open -> active, valid and closed -> finished.
-- diagnosis.condition points at the Condition of the same DBC (condition.sql).
-- Template var .patient (FHIR patient param, pushdown in config/queries/sources/sim/sim.yaml).

-- ── Statement 1: Root EpisodeOfCare ───────────────────────────────────────
SELECT
    SubtrajectNr                            AS resource_id,
    SubtrajectNr                            AS id,
    ''                                      AS parent_id,
    'EpisodeOfCare'                         AS fhir_path,
    CASE
        WHEN LOWER(DBCgeldig) IN ('false', '0', 'nee', 'n') THEN 'entered-in-error'
        WHEN DBCSluitingsDatum IS NULL OR DBCSluitingsDatum = '' THEN 'active'
        ELSE 'finished'
    END                                     AS status,
    'Patient/' || Identificatienummer       AS "patient.reference",
    NULLIF(DBCOpeningsDatum, '')            AS "period.start",
    NULLIF(DBCSluitingsDatum, '')           AS "period.end"
FROM sim_dbc
WHERE 1=1
{{- if .patient}} AND Identificatienummer = '{{.patient}}'{{end}};

-- ── Statement 2: type (ZorgType) ──────────────────────────────────────────
SELECT
    SubtrajectNr                            AS resource_id,
    SubtrajectNr || '_type'                 AS id,
    SubtrajectNr                            AS parent_id,
    'EpisodeOfCare.type'                    AS fhir_path
FROM sim_dbc
WHERE ZorgTypeCode IS NOT NULL AND ZorgTypeCode != ''
{{- if .patient}} AND Identificatienummer = '{{.patient}}'{{end}};

SELECT
    SubtrajectNr                            AS resource_id,
    SubtrajectNr || '_type_coding'          AS id,
    SubtrajectNr || '_type'                 AS parent_id,
    'EpisodeOfCare.type.coding'             AS fhir_path,
    ZorgTypeCodeSysteem                     AS "system",
    ZorgTypeCode                            AS code,
    ZorgTypeOmschrijving                    AS display
FROM sim_dbc
WHERE ZorgTypeCode IS NOT NULL AND ZorgTypeCode != ''
{{- if .patient}} AND Identificatienummer = '{{.patient}}'{{end}};

-- ── Statement 3: diagnosis → the Condition of this DBC ────────────────────
SELECT
    SubtrajectNr                            AS resource_id,
    SubtrajectNr || '_diagnosis'            AS id,
    SubtrajectNr                            AS parent_id,
    'EpisodeOfCare.diagnosis'               AS fhir_path,
    'Condition/dbc-' || SubtrajectNr        AS "condition.reference",
    1                                       AS "rank"   -- also keeps diagnosis an object (one column alone is collapsed to a bare value)
FROM sim_dbc
WHERE SpecialismeDiagnoseCode IS NOT NULL AND SpecialismeDiagnoseCode != ''
{{- if .patient}} AND Identificatienummer = '{{.patient}}'{{end}};
