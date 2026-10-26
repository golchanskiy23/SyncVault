-- Связки: папка на одном устройстве ↔ папка на другом (или на Google Drive).
-- Синхронизируются только стороны одной связки; у каждой стороны своё состояние
-- в sync_file_states (node_id = 'pair<id>:a' / 'pair<id>:b').
CREATE TABLE IF NOT EXISTS sync_pairs (
    id               BIGSERIAL PRIMARY KEY,
    name             TEXT NOT NULL DEFAULT '',
    side_a           JSONB NOT NULL,
    side_b           JSONB NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_sync_at     TIMESTAMPTZ,
    last_status      TEXT NOT NULL DEFAULT 'pending',  -- pending | ok | error
    last_error       TEXT NOT NULL DEFAULT '',
    last_transferred INTEGER NOT NULL DEFAULT 0,
    last_deleted     INTEGER NOT NULL DEFAULT 0,
    last_conflicts   INTEGER NOT NULL DEFAULT 0
);

-- Базовая папка агента на его устройстве (для отображения и абсолютных путей)
ALTER TABLE sync_nodes ADD COLUMN IF NOT EXISTS root TEXT NOT NULL DEFAULT '';

-- Узлы и состояние модели «все со всеми» больше не используются: в sync_nodes остаются только агенты
DELETE FROM sync_nodes WHERE type <> 'remote_simple';
DELETE FROM sync_file_states WHERE node_id NOT LIKE 'pair%';
