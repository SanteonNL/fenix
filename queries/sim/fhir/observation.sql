-- SIM → FHIR Observation
--
-- Tables loaded from test/data/sim/ (prefix = "sim", delimiter = semicolon):
--   sim_algemenemeting
--     MetingID, Identificatienummer,
--     MetingNaamCodeSysteem, MetingNaamCode, MetingNaamOmschrijving,
--     UitslagWaarde, UitslagWaardeEenheidSysteem, UitslagWaardeEenheid,
--     UitslagCode, UitslagCodeOmschrijving,
--     MetingDatumTijd, Status
--
-- Template vars: .status (FHIR status param, pushdown in
-- config/queries/sources/sim/sim.yaml) — filters on the real Status column
-- instead of the hardcoded 'final' literal this used to have.
-- .subject (FHIR patient param, same pushdown config) — scopes to one
-- patient, used by Group/$export (cmd/fenix/fhirserver/export.go) to fire
-- this query once per member id rather than once for the whole table.

-- ── Statement 1: Root Observation ─────────────────────────────────────────
-- NULLIF on the valueQuantity.* columns: a coded-only measurement (result
-- expressed via UitslagCode, handled in statement 3 below) has no numeric
-- UitslagWaarde at all. Without this, an empty string reaches Quantity.Value
-- (*json.Number), which fails to unmarshal ("" is not a valid number
-- literal) and the whole Observation gets skipped — NULL, unlike "", is a
-- valid (absent) value for every field type here.
SELECT
    MetingID                                AS resource_id,
    MetingID                                AS id,
    ''                                      AS parent_id,
    'Observation'                           AS fhir_path,
    Status                                  AS status,
    MetingDatumTijd                         AS effectiveDateTime,
    'Patient/' || Identificatienummer       AS "subject.reference",
    NULLIF(UitslagWaarde, '')               AS "valueQuantity.value",
    NULLIF(UitslagWaardeEenheid, '')        AS "valueQuantity.unit",
    NULLIF(UitslagWaardeEenheidSysteem, '') AS "valueQuantity.system"
FROM sim_algemenemeting
WHERE 1=1
{{- if .status}} AND Status = '{{.status}}'{{end}}
{{- if .subject}} AND Identificatienummer = '{{.subject}}'{{end}};

-- ── Statement 2: code.coding (MetingNaam) ─────────────────────────────────
SELECT
    MetingID                                AS resource_id,
    MetingID || '_code'                     AS id,
    MetingID                                AS parent_id,
    'Observation.code.coding'               AS fhir_path,
    MetingNaamCodeSysteem                   AS "system",
    MetingNaamCode                          AS code,
    MetingNaamOmschrijving                  AS display
FROM sim_algemenemeting
WHERE MetingNaamCode IS NOT NULL AND MetingNaamCode != ''
{{- if .status}} AND Status = '{{.status}}'{{end}}
{{- if .subject}} AND Identificatienummer = '{{.subject}}'{{end}};

-- ── Statement 3: valueCodeableConcept.coding (coded result) ───────────────
SELECT
    MetingID                                AS resource_id,
    MetingID || '_val'                      AS id,
    MetingID                                AS parent_id,
    'Observation.valueCodeableConcept.coding' AS fhir_path,
    UitslagCodeSysteem                      AS "system",
    UitslagCode                             AS code,
    UitslagCodeOmschrijving                 AS display
FROM sim_algemenemeting
WHERE UitslagCode IS NOT NULL AND UitslagCode != ''
{{- if .status}} AND Status = '{{.status}}'{{end}}
{{- if .subject}} AND Identificatienummer = '{{.subject}}'{{end}};
