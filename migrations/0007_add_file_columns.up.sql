-- Add file_hash column to files table
ALTER TABLE files ADD COLUMN IF NOT EXISTS file_hash VARCHAR(64);

-- Add storage_node_id column to files table  
ALTER TABLE files ADD COLUMN IF NOT EXISTS storage_node_id VARCHAR(32);

-- Add file_status column to files table
ALTER TABLE files ADD COLUMN IF NOT EXISTS file_status VARCHAR(20) DEFAULT 'created';

-- Add synced_at column to files table
ALTER TABLE files ADD COLUMN IF NOT EXISTS synced_at TIMESTAMP WITH TIME ZONE;

-- Add version column to files table
ALTER TABLE files ADD COLUMN IF NOT EXISTS version INTEGER DEFAULT 1;

-- Update existing records
UPDATE files SET 
    file_hash = COALESCE(checksum_sha256, checksum_md5, 'default_hash'),
    storage_node_id = '12345678901234567890123456789012',
    file_status = 'created',
    version = 1
WHERE file_hash IS NULL;
