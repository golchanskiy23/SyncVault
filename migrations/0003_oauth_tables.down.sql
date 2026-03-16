-- Drop Google Drive sync status table
DROP TABLE IF EXISTS google_drive_sync_status;

-- Drop Google Drive files cache table
DROP TABLE IF EXISTS google_drive_files;

-- Drop OAuth tokens table
DROP TABLE IF EXISTS oauth_tokens;

-- Drop OAuth states table
DROP TABLE IF EXISTS oauth_states;

-- Drop cleanup functions
DROP FUNCTION IF EXISTS cleanup_expired_oauth_states();
DROP FUNCTION IF EXISTS cleanup_inactive_oauth_tokens();

-- Drop update timestamp function
DROP FUNCTION IF EXISTS update_oauth_tokens_updated_at();
