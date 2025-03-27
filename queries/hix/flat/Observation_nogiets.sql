-- Active: 1740476978702@@127.0.0.1@5432@development@public
SELECT 
   identificatienummer as "Patient.id",
    metingid as "resource_id",
    metingid AS id,
    '' AS parent_id,
    'Observation' AS fhir_path,
    'unknown' AS "status", -- keeping original 'finaal'
    -- Category 0 with original and additional codings
    'text' AS "category[0].text", -- original
    'http://terminology.hl7.org/CodeSystem/' AS "category[0].coding[0].system", -- original
    'tommy' AS "category[0].coding[0].code", -- original
    'tommy' AS "category[0].coding[0].display", -- original
    'http://terminology.hl7.org/CodeSystem/observation-nogiets' AS "category[0].coding[1].system", -- original
    'laboratory' AS "category[0].coding[1].code", -- original
    'Laboratory' AS "category[0].coding[1].display", -- original
    'http://terminology.hl7.org/CodeSystem/observation-category' AS "category[0].coding[2].system",
    'vital-signs' AS "category[0].coding[2].code",
    'Vital Signs' AS "category[0].coding[2].display",
    -- Category 1 with original and additional codings
    'text1' AS "category[1].text", -- original
    'http://snomed.info/sct' AS "category[1].coding[0].system", -- original
    '336602003' AS "category[1].coding[0].code", -- original
    'LaboratoryA' AS "category[1].coding[0].display", -- original
    'http://terminology.hl7.org/CodeSystem/observation-category' AS "category[1].coding[1].system",
    'exam' AS "category[1].coding[1].code",
    'Exam' AS "category[1].coding[1].display",
    -- Category 2 with additional codings
    'Patient Vitals' AS "category[2].text",
    'http://terminology.hl7.org/CodeSystem/observation-category' AS "category[2].coding[0].system",
    'survey' AS "category[2].coding[0].code",
    'Survey' AS "category[2].coding[0].display",
    'http://loinc.org' AS "category[2].coding[1].system",
    '85354-9' AS "category[2].coding[1].code",
    'Vital signs panel' AS "category[2].coding[1].display",
    -- Value Quantity with original values
    4.6 AS "valuequantity.value", -- original
    'mg/dL' AS "valuequantity.unit", -- original
    -- Subject Reference (original format
    'Patient/' || identificatienummer AS "subject.reference",
    -- Code with original and additional codings
    'http://terminology.hl7.org/CodeSystem/observation-category' AS "code.coding[0].system", -- original
    'tyy' AS "code.coding[0].code", -- original
    'Laboratory1' AS "code.coding[0].display", -- original
    'http://snomed.info/sct' AS "code.coding[1].system",
    '33747003' AS "code.coding[1].code",
    'Glucose measurement' AS "code.coding[1].display",
    -- Effective Date Time
    TO_CHAR(metingdatumtijd, 'YYYY-MM-DD') AS "effectiveDateTime"
FROM 
    observation_raw
-- WHERE   identificatienummer ?patient 
-- WHERE  <metingid> ?id
-- WHERE  <geslachstveldin HIX> ?gender (niet implementeren)
-- WHERE   datum ?date 
LIMIT 1;



-- SELECT  
--    "identificatienummer" as "Patient.id",
--     '3' as "resource_id",
--     '4' AS id,
--     '' AS parent_id,
--     'Observation' AS fhir_path,
--     'final' AS "status",
--     'text' AS "category[0].text",
--     'text1' AS "category[1].text",
--     'http://terminology.hl7.org/CodeSystem/' AS "category[0].coding[0].system",
--     'tommy' AS "category[0].coding[0].code",
--     'tommy' AS "category[0].coding[0].display",
--     'http://terminology.hl7.org/CodeSystem/observation-nogiets' AS "category[0].coding[1].system",
--     'laboratory' AS "category[0].coding[1].code",
--     'Laboratory' AS "category[0].coding[1].display",
--     'http://terminology.hl7.org/CodeSystem/observation-category' AS "category[1].coding[0].system",
--     'laboratory1' AS "category[1].coding[0].code",
--     'Laboratory1' AS "category[1].coding[0].display",
--     4.6 AS "valuequantity.value",
--     'http://unitsofmeasure.org' AS "valuequantity.system",
--     'mg/dL' AS "valuequantity.unit",
--    -- '<' AS "valuequantity.comparator",
--     'Patient/' || identificatienummer AS "subject.reference",
--    -- 'http://terminology.hl7.org/CodeSystem/v3-NullFlavor' AS "code.coding[0].system",
--     'Obsolete' AS "code.coding[0].code",
--     'Laboratory1' AS "code.coding[0].display",
--     4.6 AS "valuequantity.value"
--     --metingdatumtijd AS "effectiveDateTime"
-- FROM 
--     observation_raw
-- --WHERE identificatienummer = :Patient.id
-- limit 1 ;
