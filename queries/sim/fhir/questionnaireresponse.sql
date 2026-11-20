-- SIM VragenlijstAntwoord → FHIR QuestionnaireResponse (QuestionnaireResponseSan)
--
-- Table loaded from test/data/sim/VragenlijstAntwoord.csv (prefix "sim"):
--   sim_vragenlijstantwoord
--     VragenlijstAntwoordID, Identificatienummer, QuestionnaireResponseQuestionnaire,
--     InvulDatumTijd, QuestionnaireResponseItemLinkId,
--     AntwoordCodeSysteem, AntwoordCode, AntwoordCodeOmschrijving, AntwoordWaarde
--
-- The SIM has one record per answer. The records with the same VragenlijstAntwoordID
-- form one QuestionnaireResponse with one item per record.
-- (No semicolons in comments: FENIX splits the statements on them.)
-- Template var .subject (FHIR patient param, pushdown in config/queries/sources/sim/sim.yaml).

-- ── Statement 1: Root QuestionnaireResponse (one per VragenlijstAntwoordID) ─
SELECT
    VragenlijstAntwoordID                   AS resource_id,
    VragenlijstAntwoordID                   AS id,
    ''                                      AS parent_id,
    'QuestionnaireResponse'                 AS fhir_path,
    'completed'                             AS status,
    MAX(QuestionnaireResponseQuestionnaire) AS questionnaire,
    MAX(InvulDatumTijd)                     AS authored,
    'Patient/' || MAX(Identificatienummer)  AS "subject.reference"
FROM sim_vragenlijstantwoord
WHERE 1=1
{{- if .subject}} AND Identificatienummer = '{{.subject}}'{{end}}
GROUP BY VragenlijstAntwoordID;

-- ── Statement 2: identifier ───────────────────────────────────────────────
SELECT
    VragenlijstAntwoordID                   AS resource_id,
    VragenlijstAntwoordID || '_identifier'  AS id,
    VragenlijstAntwoordID                   AS parent_id,
    'QuestionnaireResponse.identifier'      AS fhir_path,
    'urn:santeon:sim:vragenlijstantwoord'   AS "system",
    VragenlijstAntwoordID                   AS "value"
FROM sim_vragenlijstantwoord
WHERE 1=1
{{- if .subject}} AND Identificatienummer = '{{.subject}}'{{end}}
GROUP BY VragenlijstAntwoordID;

-- ── Statement 3: item (one per answer record) ─────────────────────────────
SELECT
    VragenlijstAntwoordID                                       AS resource_id,
    VragenlijstAntwoordID || '_item_' || QuestionnaireResponseItemLinkId AS id,
    VragenlijstAntwoordID                                       AS parent_id,
    'QuestionnaireResponse.item'                                AS fhir_path,
    QuestionnaireResponseItemLinkId                             AS linkId
FROM sim_vragenlijstantwoord
WHERE 1=1
{{- if .subject}} AND Identificatienummer = '{{.subject}}'{{end}};

-- ── Statement 4: item.answer, coded ───────────────────────────────────────
SELECT
    VragenlijstAntwoordID                                       AS resource_id,
    VragenlijstAntwoordID || '_item_' || QuestionnaireResponseItemLinkId || '_answer' AS id,
    VragenlijstAntwoordID || '_item_' || QuestionnaireResponseItemLinkId AS parent_id,
    'QuestionnaireResponse.item.answer'                         AS fhir_path,
    AntwoordCodeSysteem                                         AS "valueCoding.system",
    AntwoordCode                                                AS "valueCoding.code",
    AntwoordCodeOmschrijving                                    AS "valueCoding.display"
FROM sim_vragenlijstantwoord
WHERE AntwoordCode IS NOT NULL AND AntwoordCode != ''
{{- if .subject}} AND Identificatienummer = '{{.subject}}'{{end}};

-- ── Statement 5: item.answer, numeric ─────────────────────────────────────
SELECT
    VragenlijstAntwoordID                                       AS resource_id,
    VragenlijstAntwoordID || '_item_' || QuestionnaireResponseItemLinkId || '_answer' AS id,
    VragenlijstAntwoordID || '_item_' || QuestionnaireResponseItemLinkId AS parent_id,
    'QuestionnaireResponse.item.answer'                         AS fhir_path,
    AntwoordWaarde                                              AS "valueQuantity.value",
    'http://unitsofmeasure.org'                                 AS "valueQuantity.system",
    '{score}'                                                   AS "valueQuantity.code"
FROM sim_vragenlijstantwoord
WHERE (AntwoordCode IS NULL OR AntwoordCode = '') AND AntwoordWaarde IS NOT NULL AND AntwoordWaarde != ''
{{- if .subject}} AND Identificatienummer = '{{.subject}}'{{end}};
