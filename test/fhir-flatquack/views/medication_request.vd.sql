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
          'status': status,
          'intent': intent,
          'medication_display': (medicationCodeableConcept.text),
          'authored_on': authoredOn,
          'requester': (requester.display),
          'dosage_text': dosageInstruction.list_transform (el -> el.text).as_value ()
        } AS result
      FROM
        read_json_auto(
          'C:\Users\t.hetterscheid\Repo\fenix\test\fhir-flatquack/**/*MedicationRequest*.ndjson',
          columns = {
            id: 'VARCHAR',
            subject: 'STRUCT(reference VARCHAR)',
            status: 'VARCHAR',
            intent: 'VARCHAR',
            medicationCodeableConcept: 'STRUCT(text JSON)',
            authoredOn: 'VARCHAR',
            requester: 'STRUCT(display VARCHAR)',
            dosageInstruction: 'STRUCT(text VARCHAR)[]'
          }
        )
    )
  SELECT
    result.id,
    result.patient_ref,
    result.status,
    result.intent,
    result.medication_display,
    result.authored_on,
    result.requester,
    result.dosage_text
  FROM
    transformed
) TO 'C:\Users\t.hetterscheid\Repo\fenix\test\fhir-flatquack/medication_request_flat.csv' (FORMAT CSV, DELIMITER ',', HEADER);