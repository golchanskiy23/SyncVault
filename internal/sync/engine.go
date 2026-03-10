package sync

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"
)

// FileState хранит состояние файла на узле: хеш + векторные часы
type FileState struct {
	Path      string
	Hash      string
	Size      int64
	Clock     VectorClock
	UpdatedAt time.Time
	NodeID    string
}

// StateStore хранит FileState для всех файлов всех узлов
// В продакшне — PostgreSQL, здесь — in-memory для простоты
type StateStore interface {
	Get(nodeID, path string) (*FileState, bool)
	Set(nodeID, path string, state FileState)
	All(nodeID string) []FileState
}

// InMemoryStateStore — простая in-memory реализация
type InMemoryStateStore struct {
	data map[string]map[string]FileState // nodeID → path → state
}

func NewInMemoryStateStore() *InMemoryStateStore {
	return &InMemoryStateStore{data: make(map[string]map[string]FileState)}
}

func (s *InMemoryStateStore) Get(nodeID, path string) (*FileState, bool) {
	if m, ok := s.data[nodeID]; ok {
		if st, ok := m[path]; ok {
			return &st, true
		}
	}
	return nil, false
}

func (s *InMemoryStateStore) Set(nodeID, path string, state FileState) {
	if s.data[nodeID] == nil {
		s.data[nodeID] = make(map[string]FileState)
	}
	s.data[nodeID][path] = state
}

func (s *InMemoryStateStore) All(nodeID string) []FileState {
	var result []FileState
	for _, st := range s.data[nodeID] {
		result = append(result, st)
	}
	return result
}

// ConflictStrategy определяет как разрешать конфликты
type ConflictStrategy int

const (
	KeepNewest    ConflictStrategy = iota // побеждает более новый по времени
	KeepSource                            // всегда побеждает источник
	KeepBoth                              // сохранить оба файла с суффиксом
)

// SyncEngine выполняет синхронизацию между двумя узлами
type SyncEngine struct {
	store    StateStore
	strategy ConflictStrategy
}

func NewSyncEngine(store StateStore, strategy ConflictStrategy) *SyncEngine {
	return &SyncEngine{store: store, strategy: strategy}
}

// SyncResult содержит результат синхронизации
type SyncResult struct {
	Transferred int
	Conflicts   []ConflictInfo
	Errors      []error
}

type ConflictInfo struct {
	Path       string
	SourceNode string
	TargetNode string
	Resolution string
}

// Sync синхронизирует два узла
func (e *SyncEngine) Sync(ctx context.Context, source, target Node) (*SyncResult, error) {
	result := &SyncResult{}

	log.Printf("SyncEngine: building Merkle trees for %s ↔ %s", source.ID(), target.ID())

	// 1. Получаем списки файлов
	srcFiles, err := source.ListFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list source files: %w", err)
	}
	dstFiles, err := target.ListFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list target files: %w", err)
	}

	// 2. Строим Merkle деревья
	srcTree := BuildMerkleTree(srcFiles)
	dstTree := BuildMerkleTree(dstFiles)

	log.Printf("SyncEngine: src root=%s dst root=%s", srcTree.RootHash()[:8], dstTree.RootHash()[:8])

	// 3. Если корни совпали — ничего делать не нужно
	if srcTree.RootHash() == dstTree.RootHash() {
		log.Printf("SyncEngine: trees are identical, nothing to do")
		return result, nil
	}

	// 4. Находим расходящиеся файлы
	diffPaths := Diff(srcTree, dstTree)
	log.Printf("SyncEngine: %d files differ", len(diffPaths))

	// Строим map для быстрого поиска
	srcMap := make(map[string]FileEntry, len(srcFiles))
	for _, f := range srcFiles {
		srcMap[f.Path] = f
	}
	dstMap := make(map[string]FileEntry, len(dstFiles))
	for _, f := range dstFiles {
		dstMap[f.Path] = f
	}

	// 5. Для каждого расходящегося файла применяем Vector Clock
	for _, path := range diffPaths {
		srcEntry, inSrc := srcMap[path]
		dstEntry, inDst := dstMap[path]

		srcState, hasSrcState := e.store.Get(source.ID(), path)
		dstState, hasDstState := e.store.Get(target.ID(), path)

		switch {
		case inSrc && !inDst:
			// Файл есть только в источнике → копируем в target
			if err := e.transfer(ctx, source, target, srcEntry); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("transfer %s: %w", path, err))
			} else {
				e.updateState(source.ID(), target.ID(), srcEntry, srcState)
				result.Transferred++
				log.Printf("SyncEngine: ✓ %s → %s", source.ID(), path)
			}

		case !inSrc && inDst:
			// Файл есть только в target → копируем в source
			if err := e.transfer(ctx, target, source, dstEntry); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("transfer %s: %w", path, err))
			} else {
				e.updateState(target.ID(), source.ID(), dstEntry, dstState)
				result.Transferred++
				log.Printf("SyncEngine: ✓ %s ← %s", source.ID(), path)
			}

		case inSrc && inDst:
			// Файл есть в обоих — сравниваем Vector Clock
			if !hasSrcState || !hasDstState {
				// Нет истории — используем эвристику: копируем из source в target
				if err := e.transfer(ctx, source, target, srcEntry); err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("transfer %s: %w", path, err))
				} else {
					e.updateState(source.ID(), target.ID(), srcEntry, srcState)
					result.Transferred++
				}
				continue
			}

			rel := srcState.Clock.Compare(dstState.Clock)
			switch rel {
			case After:
				// source новее → source → target
				if err := e.transfer(ctx, source, target, srcEntry); err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("transfer %s: %w", path, err))
				} else {
					e.updateState(source.ID(), target.ID(), srcEntry, srcState)
					result.Transferred++
				}
			case Before:
				// target новее → target → source
				if err := e.transfer(ctx, target, source, dstEntry); err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("transfer %s: %w", path, err))
				} else {
					e.updateState(target.ID(), source.ID(), dstEntry, dstState)
					result.Transferred++
				}
			case Concurrent:
				// Конфликт — параллельные изменения
				resolution := e.resolveConflict(ctx, source, target, srcEntry, dstEntry, srcState, dstState)
				result.Conflicts = append(result.Conflicts, ConflictInfo{
					Path:       path,
					SourceNode: source.ID(),
					TargetNode: target.ID(),
					Resolution: resolution,
				})
				log.Printf("SyncEngine: conflict %s → resolved: %s", path, resolution)
			case Equal:
				// Одинаковые clock но разные хеши — редкий случай, копируем из source
				if err := e.transfer(ctx, source, target, srcEntry); err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("transfer %s: %w", path, err))
				} else {
					result.Transferred++
				}
			}
		}
	}

	log.Printf("SyncEngine: done — transferred=%d conflicts=%d errors=%d",
		result.Transferred, len(result.Conflicts), len(result.Errors))
	return result, nil
}

func (e *SyncEngine) transfer(ctx context.Context, from, to Node, entry FileEntry) error {
	r, err := from.ReadFile(ctx, entry.Path)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	defer r.Close()
	return to.WriteFile(ctx, entry.Path, r)
}

func (e *SyncEngine) updateState(fromID, toID string, entry FileEntry, fromState *FileState) {
	now := time.Now()

	var baseClock VectorClock
	if fromState != nil {
		baseClock = fromState.Clock
	} else {
		baseClock = NewVectorClock()
	}

	newClock := baseClock.Increment(fromID)

	e.store.Set(fromID, entry.Path, FileState{
		Path:      entry.Path,
		Hash:      entry.Hash,
		Size:      entry.Size,
		Clock:     newClock,
		UpdatedAt: now,
		NodeID:    fromID,
	})
	e.store.Set(toID, entry.Path, FileState{
		Path:      entry.Path,
		Hash:      entry.Hash,
		Size:      entry.Size,
		Clock:     newClock,
		UpdatedAt: now,
		NodeID:    toID,
	})
}

func (e *SyncEngine) resolveConflict(ctx context.Context, source, target Node, srcEntry, dstEntry FileEntry, srcState, dstState *FileState) string {
	switch e.strategy {
	case KeepNewest:
		if srcState.UpdatedAt.After(dstState.UpdatedAt) {
			e.transfer(ctx, source, target, srcEntry)
			return "kept_source_newer"
		}
		e.transfer(ctx, target, source, dstEntry)
		return "kept_target_newer"

	case KeepSource:
		e.transfer(ctx, source, target, srcEntry)
		return "kept_source"

	case KeepBoth:
		// Сохраняем оба файла с суффиксом узла
		conflictEntry := FileEntry{Path: srcEntry.Path + ".conflict." + source.ID(), Hash: srcEntry.Hash, Size: srcEntry.Size}
		e.transfer(ctx, source, target, conflictEntry)
		return "kept_both"
	}
	return "unresolved"
}

// SyncAll синхронизирует все пары узлов в сети (полный граф)
func (e *SyncEngine) SyncAll(ctx context.Context, nodes []Node) ([]*SyncResult, error) {
	var results []*SyncResult
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			r, err := e.Sync(ctx, nodes[i], nodes[j])
			if err != nil {
				log.Printf("SyncEngine: error syncing %s ↔ %s: %v", nodes[i].ID(), nodes[j].ID(), err)
				continue
			}
			results = append(results, r)
		}
	}
	return results, nil
}

// ReadCloserFromReader оборачивает io.Reader в io.ReadCloser
type nopCloser struct{ io.Reader }

func (nopCloser) Close() error { return nil }
