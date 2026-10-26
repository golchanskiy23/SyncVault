-- Несколько Google аккаунтов на одного пользователя: токены различаются по account_id (email)
ALTER TABLE oauth_tokens ADD COLUMN IF NOT EXISTS account_id TEXT NOT NULL DEFAULT '';

DROP INDEX IF EXISTS idx_oauth_tokens_unique_active;
CREATE UNIQUE INDEX IF NOT EXISTS idx_oauth_tokens_unique_active
ON oauth_tokens(user_id, provider, account_id)
WHERE is_active = TRUE;
