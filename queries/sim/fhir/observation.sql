-- SIM → FHIR Observation
--
-- Tables loaded from test/data/sim/ (prefix = "sim", delimiter = semicolon):
--   sim_algemenemeting
--     MetingID, Identificatienummer,
--     MetingNaamCodeSysteem, MetingNaamCode, MetingNaamOmschrijving,
--     UitslagWaarde, UitslagWaardeEenheidSysteem, UitslagWaardeEenheid,
--     UitslagCode, UitslagCodeOmschrijving,
--     MetingDatumTijd

-- ── Statement 1: Root Observation ─────────────────────────────────────────
SELECT
    MetingID                                AS resource_id,
    MetingID                                AS id,
    ''                                      AS parent_id,
    'Observation'                           AS fhir_path,
    'final'                                 AS status,
    MetingDatumTijd                         AS effectiveDateTime,
    'Patient/' || Identificatienummer       AS "subject.reference",
    UitslagWaarde                           AS "valueQuantity.value",
    UitslagWaardeEenheid                    AS "valueQuantity.unit",
    UitslagWaardeEenheidSysteem             AS "valueQuantity.system"
FROM sim_algemenemeting;

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
WHERE MetingNaamCode IS NOT NULL AND MetingNaamCode != '';

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
WHERE UitslagCode IS NOT NULL AND UitslagCode != '';
