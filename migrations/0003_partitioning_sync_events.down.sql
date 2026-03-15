DROP FUNCTION IF EXISTS run_scheduled_maintenance();
DROP TABLE IF EXISTS maintenance_schedule;
DROP FUNCTION IF EXISTS drop_old_partitions();
DROP FUNCTION IF EXISTS create_monthly_partitions();

CREATE TABLE IF NOT EXISTS sync_events_partitioned_backup AS 
SELECT * FROM sync_events;

DROP INDEX IF EXISTS idx_sync_events_storage_node_global;
DROP INDEX IF EXISTS idx_sync_events_file_id_global;
DROP INDEX IF EXISTS idx_sync_events_job_id_global;

DO $$
DECLARE
    partition_record RECORD;
BEGIN
    FOR partition_record IN 
        SELECT table_name 
        FROM information_schema.tables 
        WHERE table_name LIKE 'sync_events_%' 
        AND table_name > 'sync_events_'
    LOOP
        EXECUTE 'DROP TABLE ' || partition_record.table_name;
        RAISE NOTICE 'Dropped partition: %', partition_record.table_name;
    END LOOP;
END $$;

CREATE TABLE sync_events (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT REFERENCES sync_jobs(id) ON DELETE CASCADE,
    file_id BIGINT REFERENCES files(id) ON DELETE CASCADE,
    storage_node_id BIGINT REFERENCES storage_nodes(id),
    event_type VARCHAR(20) NOT NULL CHECK (event_type IN ('created', 'updated', 'deleted', 'copied', 'moved', 'synced', 'error')),
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'in_progress', 'completed', 'failed')),
    source_path TEXT,
    destination_path TEXT,
    file_size_bytes BIGINT,
    bytes_transferred BIGINT DEFAULT 0,
    transfer_rate_mbps DECIMAL(10,2),
    error_code VARCHAR(50),
    error_message TEXT,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE
);

INSERT INTO sync_events (
    id, job_id, file_id, storage_node_id, event_type, status,
    source_path, destination_path, file_size_bytes, bytes_transferred,
    transfer_rate_mbps, error_code, error_message, metadata,
    created_at, completed_at
)
SELECT 
    id, job_id, file_id, storage_node_id, event_type, status,
    source_path, destination_path, file_size_bytes, bytes_transferred,
    transfer_rate_mbps, error_code, error_message, metadata,
    created_at, completed_at
FROM sync_events_partitioned_backup
ORDER BY created_at;

CREATE INDEX idx_sync_events_job_id ON sync_events (job_id);
CREATE INDEX idx_sync_events_file_id ON sync_events (file_id);
CREATE INDEX idx_sync_events_storage_node ON sync_events (storage_node_id);
CREATE INDEX idx_sync_events_type ON sync_events (event_type);
CREATE INDEX idx_sync_events_status ON sync_events (status);
CREATE INDEX idx_sync_events_created_at ON sync_events (created_at);
CREATE INDEX idx_sync_events_created_status ON sync_events (created_at DESC, status);
CREATE INDEX idx_sync_events_metadata_gin ON sync_events USING gin (metadata);

DROP TABLE IF EXISTS sync_events_partitioned_backup;
DROP TABLE IF EXISTS sync_events_backup;
