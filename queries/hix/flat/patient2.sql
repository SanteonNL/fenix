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
        AND p.identificatienummer = :identifier
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

-- -- Additional queries (also using :identifier)

-- SELECT
--     'Patient.identifier.type.coding' as field_name,
--     '123435465'as parent_id,
--     '1234' as id,
--     'http://terminology.hl7.org/CodeSystem/v2-0203' as system,
--     'MR' as code;

-- SELECT
--     'Patient.identifier.type.coding' as field_name,
--     '123'as parent_id,
--     '12345' as id,
--     'http://terminology.hl7.org/CodeSystem/v2-0203' as system,
--     'AN' as code;

-- WITH names AS (
--     SELECT
--         'Patient.name' as field_name,
--         p.identificatienummer as parent_id,
--         concat(p.identificatienummer, humanName.lastname) AS id,
--         humanName.lastname as family,
--         JSON_ARRAY(humanName.firstname, 'Tommy', 'Jantine') AS given,
--         null as start,
--         null as end
--     FROM
--         patient p
--         JOIN names humanName ON humanName.identificatienummer = p.identificatienummer
--     WHERE
--         1 = 1
--         AND p.identificatienummer = :identifier
--     GROUP BY
--         p.identificatienummer,
--         humanName.lastname,
--         humanName.firstname
-- )
-- SELECT
--     *
-- FROM
--     names;

-- WITH names AS (
--     SELECT
--         'Patient.name' as field_name,
--         p.identificatienummer as parent_id,
--         CONCAT(
--             p.identificatienummer,
--             humanName.lastname,
--             humanName.period_start,
--             ROW_NUMBER() OVER (
--                 ORDER BY
--                     p.identificatienummer,
--                     humanName.lastname,
--                     humanName.period_start
--             )
--         ) AS id,
--         humanName.lastname as family,
--         humanName.firstname AS name,
--         period_start as start,
--         period_end as endx
--     FROM
--         patient p
--         JOIN names humanName ON humanName.identificatienummer = p.identificatienummer
--     WHERE
--         1 = 1
--         AND p.identificatienummer = :identifier
--     GROUP BY
--         p.identificatienummer,
--         humanName.lastname,
--         humanName.firstname,
--         humanName.period_start,
--         humanName.period_end
-- )
-- SELECT
--     'Patient.name.period' as field_name,
--     id as parent_id,
--     id,
--     start,
--     endx as end
-- FROM
--     names;

-- SELECT
--     'Patient.contact.telecom' AS field_name,
--     c.id AS parent_id,
--     CONCAT(c.id, cp.system) AS id,
--     cp.system,
--     cp.value
-- FROM
--     contacts c
--     JOIN contact_points cp ON c.id = cp.contact_id
-- WHERE
--     1 = 1
--     AND c.patient_id = :identifier
-- GROUP BY
--     cp.system,
--     cp.value,
--     c.id;

-- SELECT
--     'Patient.contact' AS field_name,
--     p.identificatienummer AS parent_id,
--     c.id AS id
-- FROM
--     patient p
--     JOIN contacts c ON c.patient_id = p.identificatienummer
-- WHERE
--     1 = 1
--     AND p.identificatienummer = :identifier;

-- SELECT
--     'Patient.contact.telecom' AS field_name,
--     c.id AS parent_id,
--     CONCAT(c.id, cp.system) AS id,
--     cp.system,
--     cp.value
-- FROM
--     contacts c
--     JOIN contact_points cp ON c.id = cp.contact_id
-- WHERE
--     1 = 1
--     AND c.patient_id = :identifier
-- GROUP BY
--     cp.system,
--     cp.value,
--     c.id;