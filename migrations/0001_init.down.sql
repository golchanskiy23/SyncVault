DROP VIEW IF EXISTS active_sync_jobs;
DROP VIEW IF EXISTS user_storage_stats;

DROP TRIGGER IF EXISTS manage_current_version_trigger ON file_versions;
DROP TRIGGER IF EXISTS update_storage_nodes_updated_at ON storage_nodes;
DROP TRIGGER IF EXISTS update_files_updated_at ON files;
DROP TRIGGER IF EXISTS update_users_updated_at ON users;

DROP FUNCTION IF EXISTS manage_current_file_version();
DROP FUNCTION IF EXISTS update_updated_at_column();

DROP TABLE IF EXISTS sync_events;
DROP TABLE IF EXISTS sync_jobs;
DROP TABLE IF EXISTS file_versions;
DROP TABLE IF EXISTS storage_nodes;
DROP TABLE IF EXISTS files;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;

DROP EXTENSION IF EXISTS "pgcrypto";
DROP EXTENSION IF EXISTS "uuid-ossp";
