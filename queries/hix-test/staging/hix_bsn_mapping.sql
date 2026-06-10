SELECT
  id_value AS bsn,
  patient_id AS hix_patient_number
FROM patient_identifier
WHERE id_system = 'http://fhir.nl/fhir/NamingSystem/bsn'
