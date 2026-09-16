-- Export batch_contracts for one run (joined with contracts) to a CSV file.
--
-- Usage from the sqlite3 prompt:
--   1. Open the metadata database:
--        sqlite3 G:\data\metadata\metadata.db
--   2. Set the run number BEFORE reading this script:
--        .parameter set @run 63
--   3. Run this script:
--        .read export_batch_contracts.sql
--
-- Settings restore: .once automatically sends output back to the console
-- after the single query; the final two lines put .mode/.headers back to
-- the sqlite3 defaults (list / off).
--
-- If you forget to set @run first, the query binds it to NULL and the CSV
-- comes out empty (no error) - just set the parameter and re-read.
--
-- Change the output file name on the .once line below if you want a
-- different path or a per-run name (e.g. batch_contracts_run_63.csv).
-- A LEFT JOIN is used so every batch_contracts row for the run is kept,
-- even if its serialNo is missing from contracts.

.mode csv
.headers on
.once 'batch_contracts_run.csv'

SELECT
    bc.runNo            AS runNo,
    bc.batchNo          AS batchNo,
    bc.listNo           AS listNo,
    bc.serialNo         AS serialNo,
    c.underlying        AS underlying,
    c.expiry            AS expiry,
    c.type              AS optionType,
    c.strike            AS strike,
    c.firstSeenDate     AS firstSeenDate,
    c.lastDownloadedDate AS lastDownloadedDate,
    c.downloadAttempts  AS downloadAttempts,
    c.archived          AS archived,
    bc.barCount         AS batchBarCount,
    bc.newBarCount      AS batchNewBarCount
FROM batch_contracts bc
LEFT JOIN contracts c USING (serialNo)
WHERE bc.runNo = @run
ORDER BY bc.batchNo, bc.listNo;

-- Restore the default prompt settings.
.mode list
.headers off