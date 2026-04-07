CREATE OR REPLACE MACRO as_list (a) AS if (a IS NULL, [], [a]);
CREATE OR REPLACE MACRO ifnull2 (a, b) AS ifnull (a, b);
CREATE OR REPLACE MACRO slice (a, i) AS a[i];
CREATE OR REPLACE MACRO is_false (a) AS a = false;
CREATE OR REPLACE MACRO is_true (a) AS a = true;
CREATE OR REPLACE MACRO is_null (a) AS a IS NULL;
CREATE OR REPLACE MACRO is_not_null (a) AS a IS NOT NULL;
CREATE OR REPLACE MACRO as_value (a) AS if (
  len(a) > 1,
  error('unexpected collection returned'),
  a[1]
);
COPY (
  WITH
    transformed AS (
      SELECT
        {
          'id': id,
          'gender': gender,
          'birth_date': birthDate,
          'family_name': name.list_filter (el -> ((el.use) = 'official')).list_transform (el -> el.family).as_value (),
          'given_name': name.list_filter (el -> ((el.use) = 'official')).list_transform (el -> el.given).flatten ().slice (1),
          'city': address.list_transform (el -> el.city).as_value (),
          'postal_code': address.list_transform (el -> el.postalCode).as_value (),
          'country': address.list_transform (el -> el.country).as_value (),
          'marital_status': (maritalStatus.coding).list_transform (el -> el.code).as_value ()
        } AS result
      FROM
        read_json_auto(
          'C:\Users\t.hetterscheid\Repo\fenix/**/*Patient*.ndjson',
          columns = {
            id: 'VARCHAR',
            gender: 'VARCHAR',
            birthDate: 'VARCHAR',
            name: 'STRUCT(use VARCHAR, family VARCHAR, given VARCHAR[])[]',
            address: 'STRUCT(city VARCHAR, postalCode VARCHAR, country VARCHAR)[]',
            maritalStatus: 'STRUCT(coding STRUCT(code VARCHAR)[])'
          }
        )
    )
  SELECT
    result.id,
    result.gender,
    result.birth_date,
    result.family_name,
    result.given_name,
    result.city,
    result.postal_code,
    result.country,
    result.marital_status
  FROM
    transformed
) TO 'C:\Users\t.hetterscheid\Repo\fenix/patient_flat.csv' (FORMAT CSV, DELIMITER ',', HEADER);