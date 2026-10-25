-- Single-column target indexes on wisp_dependencies: the wisp twin of the
-- durable dependencies table's idx_dep_{issue,wisp,external}_target
-- (main 0041). The reverse-dependency lookups predicate on ONE typed target
-- column at a time (sqlbuild.DepTargetSelectSQL), so each column needs an
-- index to seek. Measured on the reference Dolt before this migration:
-- depends_on_issue_id and depends_on_wisp_id reach an IndexedTableAccess only
-- through the foreign-key backing indexes fk_wisp_dep_*_target, and
-- depends_on_external has no index at all — that branch was a full Table scan.
-- Depending on an FK's backing index also makes the plan hostage to the
-- constraint (migrations/README.md: "A foreign key holds its backing index
-- hostage"), so the issue and wisp indexes are named explicitly too.
--
-- The ignored chain runs per clone with its own cursor, so both fresh clones
-- (which materialize wisp_dependencies via ignored/0001) and already-upgraded
-- workspaces pick this up. Guarded per index like ignored/0006 and ignored/0014:
-- create only when the table exists and the index is absent.
SET @needs_index = IF(
    (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES
        WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'wisp_dependencies') > 0
    AND
    (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = 'wisp_dependencies'
          AND INDEX_NAME = 'idx_wisp_dep_issue_target') = 0,
    1, 0
);
SET @sql = IF(@needs_index = 1,
    'CREATE INDEX idx_wisp_dep_issue_target ON wisp_dependencies(depends_on_issue_id)',
    'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @needs_index = IF(
    (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES
        WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'wisp_dependencies') > 0
    AND
    (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = 'wisp_dependencies'
          AND INDEX_NAME = 'idx_wisp_dep_wisp_target') = 0,
    1, 0
);
SET @sql = IF(@needs_index = 1,
    'CREATE INDEX idx_wisp_dep_wisp_target ON wisp_dependencies(depends_on_wisp_id)',
    'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @needs_index = IF(
    (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES
        WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'wisp_dependencies') > 0
    AND
    (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
        WHERE TABLE_SCHEMA = DATABASE()
          AND TABLE_NAME = 'wisp_dependencies'
          AND INDEX_NAME = 'idx_wisp_dep_external_target') = 0,
    1, 0
);
SET @sql = IF(@needs_index = 1,
    'CREATE INDEX idx_wisp_dep_external_target ON wisp_dependencies(depends_on_external)',
    'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
