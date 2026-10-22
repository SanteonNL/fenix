-- SIM patient-number query (DSG patient-id strategy, see internal/patientnumbers).
--
-- One row per known patient number:
--   number        the patient number
--   replaced_by   the number that replaced it (empty/NULL when the number is active)
--   test_patient  'true' for a test patient (optional column). As in the DSG, a
--                 flagged number is left out, and so are all numbers of a patient
--                 whose active number is flagged.
-- A patient can have many old numbers; following replaced_by always ends at the
-- one active number.
--
-- Table loaded from test/data/sim/PatientNumbers.csv (prefix = "sim"):
--   sim_patientnumbers  Identificatienummer, ReplacedBy, TestPatient
SELECT
    Identificatienummer AS number,
    ReplacedBy          AS replaced_by,
    TestPatient         AS test_patient
FROM sim_patientnumbers
