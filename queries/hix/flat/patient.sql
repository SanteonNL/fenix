WITH names AS (
    SELECT
        'Patient' as field_name,
        p.identificatienummer as id,
        '' as parent_id,
        p.geboortedatum as birthDate,
        null as system,
        null as value,
        p.geslachtcode as gender
    FROM
        patient p
    WHERE
        1 = 1
        AND p.identificatienummer =  :id
)
SELECT
    *
FROM
    names
UNION ALL
SELECT
    'Patient.identifier' as field_name,
    id as id,
    id as parent_id,
    null as birthDate,
    'https://santeon.nl' as system,
    id as value,
    null as gender
FROM
    names
UNION ALL
SELECT
    'Patient.identifier.type' as field_name,
    id,
    id as parent_id,
    null as birthDate,
    null as system,
    null as value,
    null as gender
FROM
    names
UNION ALL
SELECT
    'Patient.identifier' as field_name,
    '12345' as id,
    id as parent_id,
    null as birthDate,
    'https://santeon.nl' as system,
    '123456' as value,
    null as gender
FROM
    names
UNION ALL
SELECT
    'Patient.identifier.type' as field_nam,
    '123435465' as id,
    '12345' as parent_id,
    null as birthDate,
    null as system,
    null as value,
    null as gender
FROM
    names;

