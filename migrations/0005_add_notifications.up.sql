CREATE TABLE notification_types (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) UNIQUE NOT NULL,
    description TEXT,
    default_enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE user_notification_preferences (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_type_id INTEGER NOT NULL REFERENCES notification_types(id) ON DELETE CASCADE,
    is_enabled BOOLEAN DEFAULT true,
    email_enabled BOOLEAN DEFAULT true,
    push_enabled BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(user_id, notification_type_id)
);

CREATE TABLE notifications (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_type_id INTEGER NOT NULL REFERENCES notification_types(id),
    title VARCHAR(255) NOT NULL,
    message TEXT NOT NULL,
    data JSONB,
    is_read BOOLEAN DEFAULT false,
    is_email_sent BOOLEAN DEFAULT false,
    is_push_sent BOOLEAN DEFAULT false,
    read_at TIMESTAMP WITH TIME ZONE,
    email_sent_at TIMESTAMP WITH TIME ZONE,
    push_sent_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE notification_delivery_attempts (
    id BIGSERIAL PRIMARY KEY,
    notification_id BIGINT NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    delivery_type VARCHAR(20) NOT NULL CHECK (delivery_type IN ('email', 'push', 'sms')),
    status VARCHAR(20) NOT NULL CHECK (status IN ('pending', 'sent', 'failed', 'retry')),
    error_message TEXT,
    attempt_count INTEGER DEFAULT 1,
    next_attempt_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notification_types_name ON notification_types (name);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_user_notification_prefs_user_id ON user_notification_preferences (user_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_user_notification_prefs_type_id ON user_notification_preferences (notification_type_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_user_notification_prefs_enabled ON user_notification_preferences (is_enabled);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notifications_user_id ON notifications (user_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notifications_type_id ON notifications (notification_type_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notifications_is_read ON notifications (is_read);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notifications_created_at ON notifications (created_at);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notifications_expires_at ON notifications (expires_at);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notifications_unread ON notifications (user_id, is_read, created_at DESC) WHERE is_read = false;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_delivery_attempts_notification_id ON notification_delivery_attempts (notification_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_delivery_attempts_status ON notification_delivery_attempts (status);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_delivery_attempts_next_attempt ON notification_delivery_attempts (next_attempt_at) WHERE status = 'retry';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notifications_data_gin ON notifications USING gin (data);

CREATE TRIGGER update_user_notification_prefs_updated_at BEFORE UPDATE ON user_notification_preferences
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE OR REPLACE FUNCTION create_notification(
    p_user_id BIGINT,
    p_notification_type VARCHAR(50),
    p_title VARCHAR(255),
    p_message TEXT,
    p_data JSONB DEFAULT NULL,
    p_expires_hours INTEGER DEFAULT NULL
)
RETURNS BIGINT AS $$
DECLARE
    v_type_id INTEGER;
    v_pref_enabled BOOLEAN;
    v_notification_id BIGINT;
    v_expires_at TIMESTAMP WITH TIME ZONE;
BEGIN
    SELECT id INTO v_type_id
    FROM notification_types
    WHERE name = p_notification_type;
    
    IF v_type_id IS NULL THEN
        RAISE EXCEPTION 'Notification type % not found', p_notification_type;
    END IF;
    
    SELECT unp.is_enabled INTO v_pref_enabled
    FROM user_notification_preferences unp
    WHERE unp.user_id = p_user_id 
    AND unp.notification_type_id = v_type_id;
    
    IF v_pref_enabled IS NULL THEN
        SELECT nt.default_enabled INTO v_pref_enabled
        FROM notification_types nt
        WHERE nt.id = v_type_id;
    END IF;
    
    IF NOT v_pref_enabled THEN
        RETURN NULL;
    END IF;
    
    IF p_expires_hours IS NOT NULL THEN
        v_expires_at := CURRENT_TIMESTAMP + (p_expires_hours || ' hours')::INTERVAL;
    END IF;
    
    INSERT INTO notifications (
        user_id, notification_type_id, title, message, data, expires_at
    ) VALUES (
        p_user_id, v_type_id, p_title, p_message, p_data, v_expires_at
    ) RETURNING id INTO v_notification_id;
    
    INSERT INTO notification_delivery_attempts (notification_id, delivery_type, status, next_attempt_at)
    SELECT 
        v_notification_id,
        dt.delivery_type,
        'pending',
        CASE 
            WHEN dt.delivery_type = 'email' AND unp.email_enabled THEN CURRENT_TIMESTAMP
            WHEN dt.delivery_type = 'push' AND unp.push_enabled THEN CURRENT_TIMESTAMP
            ELSE NULL
        END
    FROM unnest(ARRAY['email', 'push']) AS dt(delivery_type)
    LEFT JOIN user_notification_preferences unp ON unp.user_id = p_user_id AND unp.notification_type_id = v_type_id
    WHERE (dt.delivery_type = 'email' AND unp.email_enabled) 
       OR (dt.delivery_type = 'push' AND unp.push_enabled);
    
    RETURN v_notification_id;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION mark_notification_read(p_notification_id BIGINT, p_user_id BIGINT)
RETURNS BOOLEAN AS $$
DECLARE
    updated_count INTEGER;
BEGIN
    UPDATE notifications 
    SET is_read = true, read_at = CURRENT_TIMESTAMP
    WHERE id = p_notification_id 
    AND user_id = p_user_id 
    AND is_read = false;
    
    GET DIAGNOSTICS updated_count = ROW_COUNT;
    RETURN updated_count > 0;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION mark_all_notifications_read(p_user_id BIGINT)
RETURNS INTEGER AS $$
DECLARE
    updated_count INTEGER;
BEGIN
    UPDATE notifications 
    SET is_read = true, read_at = CURRENT_TIMESTAMP
    WHERE user_id = p_user_id 
    AND is_read = false;
    
    GET DIAGNOSTICS updated_count = ROW_COUNT;
    RETURN updated_count;
END;
$$ LANGUAGE plpgsql;

INSERT INTO notification_types (name, description) VALUES
('sync_completed', 'File synchronization completed successfully'),
('sync_failed', 'File synchronization failed'),
('quota_warning', 'Storage quota usage warning'),
('quota_exceeded', 'Storage quota exceeded'),
('file_shared', 'File has been shared with you'),
('job_completed', 'Background job completed'),
('system_maintenance', 'System maintenance notification'),
('security_alert', 'Security-related alert')
ON CONFLICT (name) DO NOTHING;

INSERT INTO user_notification_preferences (user_id, notification_type_id, is_enabled, email_enabled, push_enabled)
SELECT 
    u.id,
    nt.id,
    nt.default_enabled,
    CASE WHEN nt.name IN ('sync_completed', 'sync_failed', 'quota_warning', 'quota_exceeded') THEN true ELSE false END,
    CASE WHEN nt.name IN ('sync_completed', 'sync_failed', 'job_completed') THEN true ELSE false END
FROM users u
CROSS JOIN notification_types nt
ON CONFLICT (user_id, notification_type_id) DO NOTHING;

CREATE VIEW unread_notifications AS
SELECT 
    n.id,
    n.user_id,
    nt.name as notification_type,
    n.title,
    n.message,
    n.data,
    n.created_at,
    n.expires_at
FROM notifications n
JOIN notification_types nt ON n.notification_type_id = nt.id
WHERE n.is_read = false
AND (n.expires_at IS NULL OR n.expires_at > CURRENT_TIMESTAMP)
ORDER BY n.created_at DESC;

CREATE VIEW notification_statistics AS
SELECT 
    u.id as user_id,
    u.username,
    COUNT(*) as total_notifications,
    COUNT(CASE WHEN n.is_read = false THEN 1 END) as unread_count,
    COUNT(CASE WHEN n.is_email_sent = true THEN 1 END) as email_sent_count,
    COUNT(CASE WHEN n.is_push_sent = true THEN 1 END) as push_sent_count,
    MAX(n.created_at) as last_notification,
    COUNT(CASE WHEN n.created_at > CURRENT_TIMESTAMP - INTERVAL '7 days' THEN 1 END) as notifications_last_week,
    COUNT(CASE WHEN n.created_at > CURRENT_TIMESTAMP - INTERVAL '30 days' THEN 1 END) as notifications_last_month
FROM users u
LEFT JOIN notifications n ON u.id = n.user_id
GROUP BY u.id, u.username;

CREATE OR REPLACE FUNCTION cleanup_expired_notifications()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM notifications 
    WHERE expires_at IS NOT NULL 
    AND expires_at < CURRENT_TIMESTAMP 
    AND is_read = true;
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION cleanup_old_read_notifications()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
    cutoff_date TIMESTAMP WITH TIME ZONE := CURRENT_TIMESTAMP - INTERVAL '90 days';
BEGIN
    DELETE FROM notifications 
    WHERE is_read = true 
    AND read_at < cutoff_date;
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

INSERT INTO maintenance_schedule (task_name, schedule_expression, next_run) VALUES
('cleanup_expired_notifications', '0 5 * * *', CURRENT_TIMESTAMP + INTERVAL '5 hours'),
('cleanup_old_read_notifications', '0 6 * * 0', CURRENT_TIMESTAMP + INTERVAL '6 hours')
ON CONFLICT DO NOTHING;
