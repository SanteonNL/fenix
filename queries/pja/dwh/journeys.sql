-- DWH cleaned layer for PJA journeys.
-- Materialized (not a view) so the prefix strip runs once and patient_id_clean is indexable.
DROP TABLE IF EXISTS dwh_pja_journeys;

CREATE TABLE dwh_pja_journeys AS
SELECT
    *,
    -- strip up to the first hyphen, e.g. 'MZH-36274752' becomes '36274752' (plain ids pass through)
    CASE
        WHEN instr(patient_id, '-') > 0
            THEN substr(patient_id, instr(patient_id, '-') + 1)
        ELSE patient_id
    END AS patient_id_clean
FROM pja_journeys;

-- per-patient lookups hit the index instead of scanning the whole table
CREATE INDEX idx_dwh_pja_journeys_patient_id_clean ON dwh_pja_journeys(patient_id_clean);
