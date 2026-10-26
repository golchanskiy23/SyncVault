package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgStateStore — персистентный StateStore на PostgreSQL.
// Переживает перезапуски сервиса, хранит VectorClock для каждого файла на каждом узле.
// Таблица sync_file_states создаётся миграцией 0009.
type PgStateStore struct {
	db *pgxpool.Pool
}

func NewPgStateStore(db *pgxpool.Pool) *PgStateStore {
	return &PgStateStore{db: db}
}

// Check проверяет, что таблицы синхронизации существуют (миграции применены)
func (s *PgStateStore) Check(ctx context.Context) error {
	for _, table := range []string{"sync_file_states", "sync_nodes", "sync_pairs"} {
		var exists bool
		if err := s.db.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("table %s does not exist — apply migrations: go run ./tools/migrate up", table)
		}
	}
	return nil
}

func (s *PgStateStore) Get(nodeID, path string) (*FileState, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var st FileState
	var clockJSON []byte

	err := s.db.QueryRow(ctx, `
		SELECT node_id, path, hash, size, clock, deleted, updated_at
		FROM sync_file_states WHERE node_id=$1 AND path=$2
	`, nodeID, path).Scan(&st.NodeID, &st.Path, &st.Hash, &st.Size, &clockJSON, &st.Deleted, &st.UpdatedAt)

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

	_, err := s.db.Exec(ctx, `
		INSERT INTO sync_file_states (node_id, path, hash, size, clock, deleted, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (node_id, path) DO UPDATE SET
			hash       = EXCLUDED.hash,
			size       = EXCLUDED.size,
			clock      = EXCLUDED.clock,
			deleted    = EXCLUDED.deleted,
			updated_at = EXCLUDED.updated_at
	`, nodeID, path, state.Hash, state.Size, clockJSON, state.Deleted, state.UpdatedAt)
	if err != nil {
		log.Printf("PgStateStore: failed to save state %s:%s: %v", nodeID, path, err)
	}
}

func (s *PgStateStore) All(nodeID string) []FileState {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := s.db.Query(ctx, `
		SELECT node_id, path, hash, size, clock, deleted, updated_at
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
		if err := rows.Scan(&st.NodeID, &st.Path, &st.Hash, &st.Size, &clockJSON, &st.Deleted, &st.UpdatedAt); err != nil {
			continue
		}
		if err := json.Unmarshal(clockJSON, &st.Clock); err != nil {
			st.Clock = NewVectorClock()
		}
		result = append(result, st)
	}
	return result
}

func (s *PgStateStore) DeleteNode(nodeID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := s.db.Exec(ctx, `DELETE FROM sync_file_states WHERE node_id=$1`, nodeID); err != nil {
		log.Printf("PgStateStore: failed to delete states of node %s: %v", nodeID, err)
	}
}

// =============================================================================
// PgNodeStore — зарегистрированные узлы в таблице sync_nodes
// =============================================================================

type PgNodeStore struct {
	db *pgxpool.Pool
}

func NewPgNodeStore(db *pgxpool.Pool) *PgNodeStore {
	return &PgNodeStore{db: db}
}

func (s *PgNodeStore) SaveNode(ctx context.Context, info NodeInfo) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO sync_nodes (id, type, endpoint, account_id, user_id, folder_id, root)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			type       = EXCLUDED.type,
			endpoint   = EXCLUDED.endpoint,
			account_id = EXCLUDED.account_id,
			user_id    = EXCLUDED.user_id,
			folder_id  = EXCLUDED.folder_id,
			root       = EXCLUDED.root,
			updated_at = NOW()
	`, info.ID, info.Type, info.Endpoint, info.AccountID, info.UserID, info.FolderID, info.Root)
	return err
}

func (s *PgNodeStore) DeleteNode(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM sync_nodes WHERE id=$1`, id)
	return err
}

func (s *PgNodeStore) LoadNodes(ctx context.Context) ([]NodeInfo, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, type, endpoint, account_id, user_id, folder_id, root
		FROM sync_nodes ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []NodeInfo
	for rows.Next() {
		var info NodeInfo
		if err := rows.Scan(&info.ID, &info.Type, &info.Endpoint, &info.AccountID, &info.UserID, &info.FolderID, &info.Root); err != nil {
			return nil, err
		}
		result = append(result, info)
	}
	return result, rows.Err()
}

// =============================================================================
// PgPairStore — связки в таблице sync_pairs (миграция 0010)
// =============================================================================

type PgPairStore struct {
	db *pgxpool.Pool
}

func NewPgPairStore(db *pgxpool.Pool) *PgPairStore {
	return &PgPairStore{db: db}
}

const pairColumns = `id, name, side_a, side_b, created_at, last_sync_at, last_status, last_error,
	last_transferred, last_deleted, last_conflicts`

func scanPair(row pgx.Row) (*Pair, error) {
	var p Pair
	var a, b []byte
	if err := row.Scan(&p.ID, &p.Name, &a, &b, &p.CreatedAt, &p.LastSyncAt, &p.LastStatus, &p.LastError,
		&p.LastTransferred, &p.LastDeleted, &p.LastConflicts); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(a, &p.A); err != nil {
		return nil, fmt.Errorf("pair %d side a: %w", p.ID, err)
	}
	if err := json.Unmarshal(b, &p.B); err != nil {
		return nil, fmt.Errorf("pair %d side b: %w", p.ID, err)
	}
	return &p, nil
}

func (s *PgPairStore) CreatePair(ctx context.Context, p *Pair) error {
	a, _ := json.Marshal(p.A)
	b, _ := json.Marshal(p.B)
	return s.db.QueryRow(ctx, `
		INSERT INTO sync_pairs (name, side_a, side_b, last_status)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`, p.Name, a, b, p.LastStatus).Scan(&p.ID, &p.CreatedAt)
}

func (s *PgPairStore) ListPairs(ctx context.Context) ([]Pair, error) {
	rows, err := s.db.Query(ctx, `SELECT `+pairColumns+` FROM sync_pairs ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Pair
	for rows.Next() {
		p, err := scanPair(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *p)
	}
	return result, rows.Err()
}

func (s *PgPairStore) GetPair(ctx context.Context, id int64) (*Pair, error) {
	p, err := scanPair(s.db.QueryRow(ctx, `SELECT `+pairColumns+` FROM sync_pairs WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPairNotFound
	}
	return p, err
}

func (s *PgPairStore) DeletePair(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM sync_pairs WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrPairNotFound
	}
	return nil
}

func (s *PgPairStore) SavePairResult(ctx context.Context, id int64, at time.Time, status, errMsg string, res *SyncResult) error {
	var transferred, deleted, conflicts int
	if res != nil {
		transferred, deleted, conflicts = res.Transferred, res.Deleted, len(res.Conflicts)
	}
	_, err := s.db.Exec(ctx, `
		UPDATE sync_pairs SET last_sync_at=$2, last_status=$3, last_error=$4,
			last_transferred=$5, last_deleted=$6, last_conflicts=$7
		WHERE id=$1
	`, id, at, status, errMsg, transferred, deleted, conflicts)
	return err
}
