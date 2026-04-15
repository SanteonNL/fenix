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
          'status': status,
          'patient_ref': (subject.reference),
          'effective_date': effectiveDateTime,
          'category': category.list_transform (el -> el.coding).flatten ().list_transform (el -> el.code).as_value (),
          'loinc_code': (code.coding).list_filter (el -> ((el.system) = 'http://loinc.org')).list_transform (el -> el.code).as_value (),
          'display': (code.text),
          'value': (valueQuantity.value),
          'unit': (valueQuantity.unit),
          'interpretation': interpretation.list_transform (el -> el.coding).flatten ().list_transform (el -> el.code).as_value ()
        } AS result
      FROM
        read_json_auto(
          'C:\Users\t.hetterscheid\Repo\fenix\test\fhir-flatquack/**/*Observation*.ndjson',
          columns = {
            id: 'VARCHAR',
            status: 'VARCHAR',
            subject: 'STRUCT(reference VARCHAR)',
            effectiveDateTime: 'VARCHAR',
            category: 'STRUCT(coding STRUCT(code VARCHAR)[])[]',
            code: 'STRUCT(coding STRUCT(system VARCHAR, code VARCHAR)[], text VARCHAR)',
            valueQuantity: 'STRUCT(value DOUBLE, unit VARCHAR)',
            interpretation: 'STRUCT(coding STRUCT(code VARCHAR)[])[]'
          }
        )
    )
  SELECT
    result.id,
    result.status,
    result.patient_ref,
    result.effective_date,
    result.category,
    result.loinc_code,
    result.display,
    result.value,
    result.unit,
    result.interpretation
  FROM
    transformed
) TO 'C:\Users\t.hetterscheid\Repo\fenix\test\fhir-flatquack/observation_flat.csv' (FORMAT CSV, DELIMITER ',', HEADER);