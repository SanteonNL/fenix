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
          'patient_ref': (subject.reference),
          'clinical_status': (clinicalStatus.coding).list_transform (el -> el.code).as_value (),
          'verification_status': (verificationStatus.coding).list_transform (el -> el.code).as_value (),
          'severity': (severity.coding).list_transform (el -> el.display).as_value (),
          'condition_display': (code.text),
          'onset_date': onsetDateTime,
          'recorded_date': recordedDate,
          'e_1': (code.coding).list_filter (el -> ((el.system) = 'http://snomed.info/sct')).list_transform (
            el -> {
              'snomed_code': (el.code),
              'snomed_display': (el.display)
            }
          ).ifnull2 ([]),
          'e_2': (code.coding).list_filter (
            el -> ((el.system) = 'http://hl7.org/fhir/sid/icd-10')
          ).list_transform (
            el -> {
              'icd10_code': (el.code),
              'icd10_display': (el.display)
            }
          ).ifnull2 ([])
        } AS result
      FROM
        read_json_auto(
          'C:\Users\t.hetterscheid\Repo\fenix/**/*Condition*.ndjson',
          columns = {
            id: 'VARCHAR',
            subject: 'STRUCT(reference VARCHAR)',
            clinicalStatus: 'STRUCT(coding STRUCT(code VARCHAR)[])',
            verificationStatus: 'STRUCT(coding STRUCT(code VARCHAR)[])',
            severity: 'STRUCT(coding STRUCT(display VARCHAR)[])',
            code: 'STRUCT(text VARCHAR, coding STRUCT(system VARCHAR, code VARCHAR, display VARCHAR)[])',
            onsetDateTime: 'VARCHAR',
            recordedDate: 'VARCHAR'
          }
        )
    )
  SELECT
    result.id,
    result.patient_ref,
    result.clinical_status,
    result.verification_status,
    result.severity,
    result.condition_display,
    result.onset_date,
    result.recorded_date,
    e_1.snomed_code,
    e_1.snomed_display,
    e_2.icd10_code,
    e_2.icd10_display
  FROM
    transformed
    CROSS JOIN UNNEST(result.e_1) AS f_8 (e_1)
    CROSS JOIN UNNEST(result.e_2) AS f_11 (e_2)
) TO 'C:\Users\t.hetterscheid\Repo\fenix/condition_flat.csv' (FORMAT CSV, DELIMITER ',', HEADER);