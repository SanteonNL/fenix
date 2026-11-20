-- SIM DBC → FHIR Condition (ConditionSan): the specialism diagnosis of a DBC.
--
-- Table loaded from test/data/sim/DBC.csv (prefix "sim"): sim_dbc.
-- id is "dbc-<SubtrajectNr>", the Condition EpisodeOfCare.diagnosis.condition points at.
-- The code system is the DBC Onderhoud OID of the specialism
-- (urn:oid:2.16.840.1.113883.2.4.3.27.15.4.<specialism without leading zeros>),
-- the code the diagnosis code alone (e.g. 922).
-- Template var .subject (FHIR patient param, pushdown in config/queries/sources/sim/sim.yaml).

-- ── Statement 1: Root Condition ───────────────────────────────────────────
SELECT
    'dbc-' || SubtrajectNr                  AS resource_id,
    'dbc-' || SubtrajectNr                  AS id,
    ''                                      AS parent_id,
    'Condition'                             AS fhir_path,
    'Patient/' || Identificatienummer       AS "subject.reference"
FROM sim_dbc
WHERE SpecialismeDiagnoseCode IS NOT NULL AND SpecialismeDiagnoseCode != ''
{{- if .subject}} AND Identificatienummer = '{{.subject}}'{{end}};

-- ── Statement 2: code.coding (SpecialismeDiagnose) ────────────────────────
SELECT
    'dbc-' || SubtrajectNr                  AS resource_id,
    'dbc-' || SubtrajectNr || '_code'       AS id,
    'dbc-' || SubtrajectNr                  AS parent_id,
    'Condition.code.coding'                 AS fhir_path,
    SpecialismeDiagnoseSysteem              AS "system",
    SpecialismeDiagnoseCode                 AS code,
    SpecialismeDiagnoseOmschrijving         AS display
FROM sim_dbc
WHERE SpecialismeDiagnoseCode IS NOT NULL AND SpecialismeDiagnoseCode != ''
{{- if .subject}} AND Identificatienummer = '{{.subject}}'{{end}};
