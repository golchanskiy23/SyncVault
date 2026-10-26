DROP INDEX IF EXISTS idx_oauth_tokens_unique_active;
ALTER TABLE oauth_tokens DROP COLUMN IF EXISTS account_id;
CREATE UNIQUE INDEX IF NOT EXISTS idx_oauth_tokens_unique_active
ON oauth_tokens(user_id, provider)
WHERE is_active = TRUE;
