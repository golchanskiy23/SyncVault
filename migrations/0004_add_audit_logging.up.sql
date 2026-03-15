CREATE TABLE audit_log (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    action VARCHAR(50) NOT NULL,
    table_name VARCHAR(50) NOT NULL,
    record_id BIGINT,
    old_values JSONB,
    new_values JSONB,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_log_user_id ON audit_log (user_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_log_action ON audit_log (action);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_log_table_name ON audit_log (table_name);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_log_created_at ON audit_log (created_at);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_log_record ON audit_log (table_name, record_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_log_old_values_gin ON audit_log USING gin (old_values);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_log_new_values_gin ON audit_log USING gin (new_values);

CREATE OR REPLACE FUNCTION audit_trigger_function()
RETURNS TRIGGER AS $$
DECLARE
    audit_action VARCHAR(50);
    old_data JSONB;
    new_data JSONB;
    current_user_id BIGINT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        audit_action := 'INSERT';
        old_data := NULL;
        new_data := to_jsonb(NEW);
    ELSIF TG_OP = 'UPDATE' THEN
        audit_action := 'UPDATE';
        old_data := to_jsonb(OLD);
        new_data := to_jsonb(NEW);
    ELSIF TG_OP = 'DELETE' THEN
        audit_action := 'DELETE';
        old_data := to_jsonb(OLD);
        new_data := NULL;
    ELSE
        RETURN NULL;
    END IF;
    
    BEGIN
        current_user_id := current_setting('app.current_user_id', true)::BIGINT;
    EXCEPTION
        WHEN OTHERS THEN
            current_user_id := NULL;
    END;
    
    INSERT INTO audit_log (
        user_id,
        action,
        table_name,
        record_id,
        old_values,
        new_values,
        ip_address,
        user_agent
    ) VALUES (
        current_user_id,
        audit_action,
        TG_TABLE_NAME,
        COALESCE(NEW.id, OLD.id),
        old_data,
        new_data,
        inet_client_addr(),
        current_setting('app.user_agent', true)
    );
    
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    ELSE
        RETURN NEW;
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER audit_users_trigger
    AFTER INSERT OR UPDATE OR DELETE ON users
    FOR EACH ROW EXECUTE FUNCTION audit_trigger_function();

CREATE TRIGGER audit_files_trigger
    AFTER INSERT OR UPDATE OR DELETE ON files
    FOR EACH ROW EXECUTE FUNCTION audit_trigger_function();

CREATE TRIGGER audit_file_versions_trigger
    AFTER INSERT OR UPDATE OR DELETE ON file_versions
    FOR EACH ROW EXECUTE FUNCTION audit_trigger_function();

CREATE TRIGGER audit_storage_nodes_trigger
    AFTER INSERT OR UPDATE OR DELETE ON storage_nodes
    FOR EACH ROW EXECUTE FUNCTION audit_trigger_function();

CREATE TRIGGER audit_sync_jobs_trigger
    AFTER INSERT OR UPDATE OR DELETE ON sync_jobs
    FOR EACH ROW EXECUTE FUNCTION audit_trigger_function();

CREATE TRIGGER audit_sessions_trigger
    AFTER INSERT OR DELETE ON sessions
    FOR EACH ROW EXECUTE FUNCTION audit_trigger_function();

CREATE VIEW user_activity_summary AS
SELECT 
    u.id as user_id,
    u.username,
    COUNT(*) as total_actions,
    COUNT(DISTINCT al.table_name) as tables_affected,
    MIN(al.created_at) as first_action,
    MAX(al.created_at) as last_action,
    COUNT(CASE WHEN al.action = 'INSERT' THEN 1 END) as insert_count,
    COUNT(CASE WHEN al.action = 'UPDATE' THEN 1 END) as update_count,
    COUNT(CASE WHEN al.action = 'DELETE' THEN 1 END) as delete_count
FROM users u
LEFT JOIN audit_log al ON u.id = al.user_id
GROUP BY u.id, u.username;

CREATE VIEW recent_activity AS
SELECT 
    al.id,
    al.action,
    al.table_name,
    al.record_id,
    u.username,
    al.created_at,
    al.ip_address,
    CASE 
        WHEN al.action = 'INSERT' THEN jsonb_extract_path_text(al.new_values, 'file_name', 'username', 'node_name')
        WHEN al.action = 'UPDATE' THEN jsonb_extract_path_text(al.new_values, 'file_name', 'username', 'node_name')
        WHEN al.action = 'DELETE' THEN jsonb_extract_path_text(al.old_values, 'file_name', 'username', 'node_name')
        ELSE NULL
    END as description
FROM audit_log al
LEFT JOIN users u ON al.user_id = u.id
ORDER BY al.created_at DESC;

CREATE VIEW file_operations_audit AS
SELECT 
    al.id,
    al.action,
    al.record_id as file_id,
    al.old_values,
    al.new_values,
    u.username,
    al.created_at,
    al.ip_address,
    CASE
        WHEN al.action = 'INSERT' THEN 'File created: ' || COALESCE(al.new_values->>'file_name', 'Unknown')
        WHEN al.action = 'UPDATE' THEN 'File updated: ' || COALESCE(al.new_values->>'file_name', al.old_values->>'file_name', 'Unknown')
        WHEN al.action = 'DELETE' THEN 'File deleted: ' || COALESCE(al.old_values->>'file_name', 'Unknown')
        ELSE al.action || ' file'
    END as operation_description
FROM audit_log al
LEFT JOIN users u ON al.user_id = u.id
WHERE al.table_name = 'files'
ORDER BY al.created_at DESC;


CREATE OR REPLACE FUNCTION cleanup_old_audit_logs()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
    cutoff_date TIMESTAMP WITH TIME ZONE := CURRENT_TIMESTAMP - INTERVAL '2 years';
BEGIN
    DELETE FROM audit_log 
    WHERE created_at < cutoff_date;
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

INSERT INTO maintenance_schedule (task_name, schedule_expression, next_run) VALUES
('cleanup_old_audit_logs', '0 4 1 * *', DATE_TRUNC('month', CURRENT_DATE + INTERVAL '1 month') + INTERVAL '4 hours')
ON CONFLICT DO NOTHING;

CREATE OR REPLACE FUNCTION get_user_activity(
    p_user_id BIGINT,
    p_start_date TIMESTAMP WITH TIME ZONE,
    p_end_date TIMESTAMP WITH TIME ZONE
)
RETURNS TABLE (
    action VARCHAR(50),
    table_name VARCHAR(50),
    record_id BIGINT,
    details JSONB,
    created_at TIMESTAMP WITH TIME ZONE
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        al.action,
        al.table_name,
        al.record_id,
        CASE 
            WHEN al.action = 'INSERT' THEN al.new_values
            WHEN al.action = 'DELETE' THEN al.old_values
            ELSE al.new_values
        END as details,
        al.created_at
    FROM audit_log al
    WHERE al.user_id = p_user_id
    AND al.created_at BETWEEN p_start_date AND p_end_date
    ORDER BY al.created_at DESC;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION get_table_activity_summary(
    p_table_name VARCHAR(50),
    p_start_date TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP - INTERVAL '7 days',
    p_end_date TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
)
RETURNS TABLE (
    action VARCHAR(50),
    action_count BIGINT,
    unique_users BIGINT,
    first_occurrence TIMESTAMP WITH TIME ZONE,
    last_occurrence TIMESTAMP WITH TIME ZONE
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        al.action,
        COUNT(*) as action_count,
        COUNT(DISTINCT al.user_id) as unique_users,
        MIN(al.created_at) as first_occurrence,
        MAX(al.created_at) as last_occurrence
    FROM audit_log al
    WHERE al.table_name = p_table_name
    AND al.created_at BETWEEN p_start_date AND p_end_date
    GROUP BY al.action
    ORDER BY action_count DESC;
END;
$$ LANGUAGE plpgsql;
