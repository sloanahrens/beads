-- Reverse migration 0067: drop the generic samples table.
--
-- REFUSES TO DESTROY OBSERVATIONS BY DEFAULT. A samples row is an observation
-- a producer cannot re-derive (an old provider balance, a usage record for a
-- run that already completed), so the table is dropped only when it is empty
-- or the operator has explicitly forced it.
--
-- Force is the session variable @bd_migrate_force = 1. bd does not execute
-- down migrations (it embeds only *.up.sql), and the SQL engine cannot read
-- the process environment, so nothing in this repo maps the operator's shell
-- variable BD_MIGRATE_FORCE to it: whoever runs this file sets the session
-- variable first, in the same session that sources it:
--
--   SET @bd_migrate_force = 1;
--   SOURCE 0067_create_samples.down.sql;
--
-- THE DECISION IS MADE INSIDE SQL, not by the client stopping on an error: the
-- statement executed below is either the DROP or a refusal, so a client that
-- keeps going after an error still cannot drop a populated table. The refusal
-- is a deliberately missing table whose name is the instruction.
--
-- KNOWN CLIENT LIMIT: `dolt sql -q` silently no-ops PREPARE/EXECUTE-driven DDL
-- (see migrations/README.md), so through that CLI this file does nothing at
-- all, including when forced. Run it through the Dolt SQL server (or a client
-- that executes prepared statements) and confirm the outcome by reading
-- INFORMATION_SCHEMA rather than an exit code.
SET @bd_migrate_force = COALESCE(@bd_migrate_force, 0);

SET @has_samples = (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'samples'
);

SET @samples_rows = 0;
SET @sql = IF(@has_samples > 0,
    'SET @samples_rows = (SELECT COUNT(*) FROM samples)',
    'SET @samples_rows = 0');
PREPARE samples_down_count FROM @sql;
EXECUTE samples_down_count;
DEALLOCATE PREPARE samples_down_count;

SET @sql = IF(@has_samples = 0 OR @samples_rows = 0 OR @bd_migrate_force = 1,
    'DROP TABLE IF EXISTS samples',
    'SELECT * FROM samples_down_refused_rows_present_set_session_bd_migrate_force');
PREPARE samples_down_drop FROM @sql;
EXECUTE samples_down_drop;
DEALLOCATE PREPARE samples_down_drop;
