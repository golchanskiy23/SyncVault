package sync

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PgStateStore — персистентный StateStore на PostgreSQL.
// Переживает перезапуски сервиса, хранит VectorClock для каждого файла на каждом узле.
type PgStateStore struct {
	db *pgxpool.Pool
}

func NewPgStateStore(db *pgxpool.Pool) *PgStateStore {
	return &PgStateStore{db: db}
}

// Migrate создаёт таблицу если не существует
func (s *PgStateStore) Migrate(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS sync_file_states (
			node_id    TEXT NOT NULL,
			path       TEXT NOT NULL,
			hash       TEXT NOT NULL,
			size       BIGINT NOT NULL DEFAULT 0,
			clock      JSONB NOT NULL DEFAULT '{}',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (node_id, path)
		)
	`)
	return err
}

func (s *PgStateStore) Get(nodeID, path string) (*FileState, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var st FileState
	var clockJSON []byte

	err := s.db.QueryRow(ctx, `
		SELECT node_id, path, hash, size, clock, updated_at
		FROM sync_file_states WHERE node_id=$1 AND path=$2
	`, nodeID, path).Scan(&st.NodeID, &st.Path, &st.Hash, &st.Size, &clockJSON, &st.UpdatedAt)

	if err != nil {
		return nil, false
	}

	if err := json.Unmarshal(clockJSON, &st.Clock); err != nil {
		st.Clock = NewVectorClock()
	}
	return &st, true
}

func (s *PgStateStore) Set(nodeID, path string, state FileState) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clockJSON, _ := json.Marshal(state.Clock)

	s.db.Exec(ctx, `
		INSERT INTO sync_file_states (node_id, path, hash, size, clock, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (node_id, path) DO UPDATE SET
			hash       = EXCLUDED.hash,
			size       = EXCLUDED.size,
			clock      = EXCLUDED.clock,
			updated_at = EXCLUDED.updated_at
	`, nodeID, path, state.Hash, state.Size, clockJSON, time.Now())
}

func (s *PgStateStore) All(nodeID string) []FileState {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := s.db.Query(ctx, `
		SELECT node_id, path, hash, size, clock, updated_at
		FROM sync_file_states WHERE node_id=$1
	`, nodeID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []FileState
	for rows.Next() {
		var st FileState
		var clockJSON []byte
		if err := rows.Scan(&st.NodeID, &st.Path, &st.Hash, &st.Size, &clockJSON, &st.UpdatedAt); err != nil {
			continue
		}
		if err := json.Unmarshal(clockJSON, &st.Clock); err != nil {
			st.Clock = NewVectorClock()
		}
		result = append(result, st)
	}
	return result
}
