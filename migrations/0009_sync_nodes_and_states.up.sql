-- Зарегистрированные узлы синхронизации (переживают перезапуск oauth-service)
CREATE TABLE IF NOT EXISTS sync_nodes (
    id          TEXT PRIMARY KEY,
    type        TEXT NOT NULL,
    endpoint    TEXT NOT NULL DEFAULT '',  -- root_path для simple, URL агента для remote_simple
    account_id  TEXT NOT NULL DEFAULT '',  -- email для google_drive
    user_id     TEXT NOT NULL DEFAULT '',  -- владелец OAuth токена для google_drive
    folder_id   TEXT NOT NULL DEFAULT '',  -- корневая папка на Drive ('' — корень)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Состояние файлов на узлах после последней синхронизации (векторные часы).
-- deleted = true — tombstone: файл был удалён и удаление распространяется на другие узлы.
CREATE TABLE IF NOT EXISTS sync_file_states (
    node_id    TEXT NOT NULL,
    path       TEXT NOT NULL,
    hash       TEXT NOT NULL DEFAULT '',
    size       BIGINT NOT NULL DEFAULT 0,
    clock      JSONB NOT NULL DEFAULT '{}',
    deleted    BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, path)
);
