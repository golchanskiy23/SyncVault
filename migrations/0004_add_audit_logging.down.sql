DROP FUNCTION IF EXISTS get_table_activity_summary(VARCHAR(50), TIMESTAMP WITH TIME ZONE, TIMESTAMP WITH TIME ZONE);
DROP FUNCTION IF EXISTS get_user_activity(BIGINT, TIMESTAMP WITH TIME ZONE, TIMESTAMP WITH TIME ZONE);

DELETE FROM maintenance_schedule WHERE task_name = 'cleanup_old_audit_logs';

DROP FUNCTION IF EXISTS cleanup_old_audit_logs();

DROP VIEW IF EXISTS file_operations_audit;
DROP VIEW IF EXISTS recent_activity;
DROP VIEW IF EXISTS user_activity_summary;

DROP TRIGGER IF EXISTS audit_sessions_trigger ON sessions;
DROP TRIGGER IF EXISTS audit_sync_jobs_trigger ON sync_jobs;
DROP TRIGGER IF EXISTS audit_storage_nodes_trigger ON storage_nodes;
DROP TRIGGER IF EXISTS audit_file_versions_trigger ON file_versions;
DROP TRIGGER IF EXISTS audit_files_trigger ON files;
DROP TRIGGER IF EXISTS audit_users_trigger ON users;

DROP FUNCTION IF EXISTS audit_trigger_function();

DROP INDEX IF EXISTS idx_audit_log_new_values_gin;
DROP INDEX IF EXISTS idx_audit_log_old_values_gin;
DROP INDEX IF EXISTS idx_audit_log_record;
DROP INDEX IF EXISTS idx_audit_log_created_at;
DROP INDEX IF EXISTS idx_audit_log_table_name;
DROP INDEX IF EXISTS idx_audit_log_action;
DROP INDEX IF EXISTS idx_audit_log_user_id;

DROP TABLE IF EXISTS audit_log;
