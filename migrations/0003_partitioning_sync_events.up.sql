CREATE TABLE IF NOT EXISTS sync_events_backup AS 
SELECT * FROM sync_events;

DROP INDEX IF EXISTS idx_sync_events_metadata_gin;
DROP INDEX IF EXISTS idx_sync_events_created_status;
DROP INDEX IF EXISTS idx_sync_events_storage_node;
DROP INDEX IF EXISTS idx_sync_events_created_at;
DROP INDEX IF EXISTS idx_sync_events_status;
DROP INDEX IF EXISTS idx_sync_events_type;
DROP INDEX IF EXISTS idx_sync_events_file_id;
DROP INDEX IF EXISTS idx_sync_events_job_id;

ALTER TABLE sync_events DROP CONSTRAINT IF EXISTS sync_events_job_id_fkey;
ALTER TABLE sync_events DROP CONSTRAINT IF EXISTS sync_events_file_id_fkey;
ALTER TABLE sync_events DROP CONSTRAINT IF EXISTS sync_events_storage_node_id_fkey;

CREATE TABLE sync_events_partitioned (
    id BIGSERIAL,
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
) PARTITION BY RANGE (created_at);

DO $$
DECLARE
    start_date DATE := DATE_TRUNC('month', CURRENT_DATE - INTERVAL '2 months');
    end_date DATE := start_date + INTERVAL '2 years';
    current_date DATE := start_date;
    partition_name TEXT;
BEGIN
    WHILE current_date < end_date LOOP
        partition_name := to_char(current_date, 'YYYY"m"MM');
        
        EXECUTE format(
            'CREATE TABLE sync_events_%s PARTITION OF sync_events_partitioned
             FOR VALUES FROM (%L) TO (%L)',
            partition_name,
            current_date,
            current_date + INTERVAL '1 month'
        );
        
        EXECUTE format(
            'CREATE INDEX idx_sync_events_%s_job_id ON sync_events_%s (job_id)',
            partition_name, partition_name
        );
        
        EXECUTE format(
            'CREATE INDEX idx_sync_events_%s_file_id ON sync_events_%s (file_id)',
            partition_name, partition_name
        );
        
        EXECUTE format(
            'CREATE INDEX idx_sync_events_%s_storage_node ON sync_events_%s (storage_node_id)',
            partition_name, partition_name
        );
        
        EXECUTE format(
            'CREATE INDEX idx_sync_events_%s_type ON sync_events_%s (event_type)',
            partition_name, partition_name
        );
        
        EXECUTE format(
            'CREATE INDEX idx_sync_events_%s_status ON sync_events_%s (status)',
            partition_name, partition_name
        );
        
        EXECUTE format(
            'CREATE INDEX idx_sync_events_%s_created_at ON sync_events_%s (created_at)',
            partition_name, partition_name
        );
        
        EXECUTE format(
            'CREATE INDEX idx_sync_events_%s_created_status ON sync_events_%s (created_at DESC, status)',
            partition_name, partition_name
        );
        
        EXECUTE format(
            'CREATE INDEX idx_sync_events_%s_metadata_gin ON sync_events_%s USING gin (metadata)',
            partition_name, partition_name
        );
        
        current_date := current_date + INTERVAL '1 month';
    END LOOP;
END $$;

INSERT INTO sync_events_partitioned (
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
FROM sync_events
ORDER BY created_at;

DROP TABLE sync_events;

ALTER TABLE sync_events_partitioned RENAME TO sync_events;

CREATE INDEX idx_sync_events_job_id_global ON sync_events (job_id);
CREATE INDEX idx_sync_events_file_id_global ON sync_events (file_id);
CREATE INDEX idx_sync_events_storage_node_global ON sync_events (storage_node_id);

CREATE OR REPLACE FUNCTION create_monthly_partitions()
RETURNS TEXT AS $$
DECLARE
    next_month_start DATE := DATE_TRUNC('month', CURRENT_DATE + INTERVAL '1 month');
    next_month_end DATE := next_month_start + INTERVAL '1 month';
    partition_name TEXT;
BEGIN
    partition_name := to_char(next_month_start, 'YYYY"m"MM');
    
    IF EXISTS (
        SELECT 1 FROM information_schema.tables 
        WHERE table_name = 'sync_events_' || partition_name
    ) THEN
        RETURN 'Partition ' || partition_name || ' already exists';
    END IF;
    
    EXECUTE format(
        'CREATE TABLE sync_events_%s PARTITION OF sync_events
         FOR VALUES FROM (%L) TO (%L)',
        partition_name,
        next_month_start,
        next_month_end
    );
    
    EXECUTE format(
        'CREATE INDEX idx_sync_events_%s_job_id ON sync_events_%s (job_id)',
        partition_name, partition_name
    );
    
    EXECUTE format(
        'CREATE INDEX idx_sync_events_%s_file_id ON sync_events_%s (file_id)',
        partition_name, partition_name
    );
    
    EXECUTE format(
        'CREATE INDEX idx_sync_events_%s_storage_node ON sync_events_%s (storage_node_id)',
        partition_name, partition_name
    );
    
    EXECUTE format(
        'CREATE INDEX idx_sync_events_%s_type ON sync_events_%s (event_type)',
        partition_name, partition_name
    );
    
    EXECUTE format(
        'CREATE INDEX idx_sync_events_%s_status ON sync_events_%s (status)',
        partition_name, partition_name
    );
    
    EXECUTE format(
        'CREATE INDEX idx_sync_events_%s_created_at ON sync_events_%s (created_at)',
        partition_name, partition_name
    );
    
    EXECUTE format(
        'CREATE INDEX idx_sync_events_%s_created_status ON sync_events_%s (created_at DESC, status)',
        partition_name, partition_name
    );
    
    EXECUTE format(
        'CREATE INDEX idx_sync_events_%s_metadata_gin ON sync_events_%s USING gin (metadata)',
        partition_name, partition_name
    );
    
    RETURN 'Created partition ' || partition_name;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION drop_old_partitions()
RETURNS TEXT AS $$
DECLARE
    cutoff_date DATE := CURRENT_DATE - INTERVAL '1 year';
    partition_name TEXT;
    dropped_count INTEGER := 0;
    partition_record RECORD;
BEGIN
    FOR partition_record IN 
        SELECT table_name 
        FROM information_schema.tables 
        WHERE table_name LIKE 'sync_events_%' 
        AND table_name > 'sync_events_'
        AND substring(table_name from 12) < to_char(cutoff_date, 'YYYY"m"MM')
    LOOP
        partition_name := partition_record.table_name;
        
        EXECUTE 'DROP TABLE ' || partition_name;
        dropped_count := dropped_count + 1;
        
        RAISE NOTICE 'Dropped partition: %', partition_name;
    END LOOP;
    
    RETURN 'Dropped ' || dropped_count || ' partitions';
END;
$$ LANGUAGE plpgsql;

CREATE TABLE IF NOT EXISTS maintenance_schedule (
    id SERIAL PRIMARY KEY,
    task_name VARCHAR(100) NOT NULL,
    last_run TIMESTAMP WITH TIME ZONE,
    next_run TIMESTAMP WITH TIME ZONE,
    is_active BOOLEAN DEFAULT true,
    schedule_expression VARCHAR(50)
);

INSERT INTO maintenance_schedule (task_name, schedule_expression, next_run) VALUES
('create_monthly_partitions', '0 0 1 * *', DATE_TRUNC('month', CURRENT_DATE + INTERVAL '1 month')),
('drop_old_partitions', '0 2 1 * *', DATE_TRUNC('month', CURRENT_DATE + INTERVAL '1 month') + INTERVAL '2 hours'),
('cleanup_old_sync_events', '0 3 * * 0', CURRENT_DATE + INTERVAL '3 hours')
ON CONFLICT DO NOTHING;

CREATE OR REPLACE FUNCTION run_scheduled_maintenance()
RETURNS TEXT AS $$
DECLARE
    task_record RECORD;
    result TEXT := '';
BEGIN
    FOR task_record IN 
        SELECT * FROM maintenance_schedule 
        WHERE is_active = true 
        AND next_run <= CURRENT_TIMESTAMP
    LOOP
        BEGIN
            CASE task_record.task_name
                WHEN 'create_monthly_partitions' THEN
                    result := result || create_monthly_partitions() || E'\n';
                WHEN 'drop_old_partitions' THEN
                    result := result || drop_old_partitions() || E'\n';
                WHEN 'cleanup_old_sync_events' THEN
                    DECLARE
                        deleted_count INTEGER;
                    BEGIN
                        deleted_count := cleanup_old_sync_events();
                        result := result || 'Cleaned up ' || deleted_count || ' old sync events' || E'\n';
                    END;
                ELSE
                    result := result || 'Unknown task: ' || task_record.task_name || E'\n';
            END CASE;
            
            UPDATE maintenance_schedule 
            SET last_run = CURRENT_TIMESTAMP,
                next_run = CASE task_name
                    WHEN 'create_monthly_partitions' THEN DATE_TRUNC('month', CURRENT_DATE + INTERVAL '1 month')
                    WHEN 'drop_old_partitions' THEN DATE_TRUNC('month', CURRENT_DATE + INTERVAL '1 month') + INTERVAL '2 hours'
                    WHEN 'cleanup_old_sync_events' THEN CURRENT_DATE + INTERVAL '7 days'
                    ELSE next_run + INTERVAL '1 day'
                END
            WHERE id = task_record.id;
            
        EXCEPTION
            WHEN OTHERS THEN
                result := result || 'Error in task ' || task_record.task_name || ': ' || SQLERRM || E'\n';
        END;
    END LOOP;
    
    RETURN COALESCE(result, 'No scheduled tasks to run');
END;
$$ LANGUAGE plpgsql;
