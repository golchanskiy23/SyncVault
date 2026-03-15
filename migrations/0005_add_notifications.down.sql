DELETE FROM maintenance_schedule WHERE task_name IN ('cleanup_expired_notifications', 'cleanup_old_read_notifications');

DROP FUNCTION IF EXISTS cleanup_old_read_notifications();
DROP FUNCTION IF EXISTS cleanup_expired_notifications();

DROP FUNCTION IF EXISTS mark_all_notifications_read(BIGINT);
DROP FUNCTION IF EXISTS mark_notification_read(BIGINT, BIGINT);
DROP FUNCTION IF EXISTS create_notification(BIGINT, VARCHAR(50), VARCHAR(255), TEXT, JSONB, INTEGER);

DROP TRIGGER IF EXISTS update_user_notification_prefs_updated_at ON user_notification_preferences;

DROP VIEW IF EXISTS notification_statistics;
DROP VIEW IF EXISTS unread_notifications;

DROP INDEX IF EXISTS idx_delivery_attempts_next_attempt;
DROP INDEX IF EXISTS idx_delivery_attempts_status;
DROP INDEX IF EXISTS idx_delivery_attempts_notification_id;

DROP INDEX IF EXISTS idx_notifications_data_gin;
DROP INDEX IF EXISTS idx_notifications_unread;
DROP INDEX IF EXISTS idx_notifications_expires_at;
DROP INDEX IF EXISTS idx_notifications_created_at;
DROP INDEX IF EXISTS idx_notifications_is_read;
DROP INDEX IF EXISTS idx_notifications_type_id;
DROP INDEX IF EXISTS idx_notifications_user_id;

DROP INDEX IF EXISTS idx_user_notification_prefs_enabled;
DROP INDEX IF EXISTS idx_user_notification_prefs_type_id;
DROP INDEX IF EXISTS idx_user_notification_prefs_user_id;

DROP INDEX IF EXISTS idx_notification_types_name;

DROP TABLE IF EXISTS notification_delivery_attempts;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS user_notification_preferences;
DROP TABLE IF EXISTS notification_types;
