package sync

import (
	"context"
	"fmt"
	"io"
	"log"
	stdsync "sync"
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
	// Deleted — tombstone: файл удалён, удаление распространяется на другие узлы
	Deleted bool
}

// StateStore хранит FileState для всех файлов всех узлов
// В продакшне — PostgreSQL, здесь — in-memory для простоты
type StateStore interface {
	Get(nodeID, path string) (*FileState, bool)
	Set(nodeID, path string, state FileState)
	All(nodeID string) []FileState
	// DeleteNode забывает состояние всех файлов узла (при удалении/перерегистрации узла)
	DeleteNode(nodeID string)
}

// InMemoryStateStore — простая in-memory реализация
type InMemoryStateStore struct {
	mu   stdsync.RWMutex
	data map[string]map[string]FileState // nodeID → path → state
}

func NewInMemoryStateStore() *InMemoryStateStore {
	return &InMemoryStateStore{data: make(map[string]map[string]FileState)}
}

func (s *InMemoryStateStore) Get(nodeID, path string) (*FileState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if m, ok := s.data[nodeID]; ok {
		if st, ok := m[path]; ok {
			return &st, true
		}
	}
	return nil, false
}

func (s *InMemoryStateStore) Set(nodeID, path string, state FileState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data[nodeID] == nil {
		s.data[nodeID] = make(map[string]FileState)
	}
	s.data[nodeID][path] = state
}

func (s *InMemoryStateStore) All(nodeID string) []FileState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []FileState
	for _, st := range s.data[nodeID] {
		result = append(result, st)
	}
	return result
}

func (s *InMemoryStateStore) DeleteNode(nodeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, nodeID)
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
	// runMu сериализует синхронизации: фоновая и ручные не должны
	// одновременно менять одни и те же узлы и их состояние
	runMu stdsync.Mutex
}

func NewSyncEngine(store StateStore, strategy ConflictStrategy) *SyncEngine {
	return &SyncEngine{store: store, strategy: strategy}
}

// SyncResult содержит результат синхронизации
type SyncResult struct {
	Transferred int
	Deleted     int
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
	e.runMu.Lock()
	defer e.runMu.Unlock()

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

	// Строим map для быстрого поиска
	srcMap := make(map[string]FileEntry, len(srcFiles))
	for _, f := range srcFiles {
		srcMap[f.Path] = f
	}
	dstMap := make(map[string]FileEntry, len(dstFiles))
	for _, f := range dstFiles {
		dstMap[f.Path] = f
	}
	srcStates := indexStates(e.store.All(source.ID()))
	dstStates := indexStates(e.store.All(target.ID()))

	// Пустой узел, на котором раньше были файлы, — скорее всего не примонтирован диск или
	// подменена папка. Без этой проверки все его файлы сочлись бы удалёнными на других узлах.
	if err := checkNotWiped(source.ID(), srcFiles, srcStates); err != nil {
		return nil, err
	}
	if err := checkNotWiped(target.ID(), dstFiles, dstStates); err != nil {
		return nil, err
	}

	// 3. Одинаковые на обоих узлах файлы: записываем состояние, если его нет или оно устарело —
	// без этого удаление такого файла на одном узле не отличить от появления нового на другом
	for path, srcEntry := range srcMap {
		dstEntry, ok := dstMap[path]
		if !ok || srcEntry.Hash != dstEntry.Hash {
			continue
		}
		srcState, dstState := srcStates[path], dstStates[path]
		if isCurrent(srcState, srcEntry) && isCurrent(dstState, dstEntry) {
			continue
		}
		e.updateState(source.ID(), target.ID(), srcEntry, srcState, dstState)
	}

	// 4. Если корни совпали — передавать нечего
	if srcTree.RootHash() == dstTree.RootHash() {
		log.Printf("SyncEngine: trees are identical, nothing to do")
		return result, nil
	}

	// 5. Находим расходящиеся файлы
	diffPaths := Diff(srcTree, dstTree)
	log.Printf("SyncEngine: %d files differ", len(diffPaths))

	// 6. Для каждого расходящегося файла применяем Vector Clock
	for _, path := range diffPaths {
		srcEntry, inSrc := srcMap[path]
		dstEntry, inDst := dstMap[path]

		srcState, hasSrcState := srcStates[path], srcStates[path] != nil
		dstState, hasDstState := dstStates[path], dstStates[path] != nil

		switch {
		case inSrc && !inDst:
			if deletedOnOther(dstState, srcState, srcEntry) {
				// target удалил файл, а source его с тех пор не менял → удаляем и на source
				if err := source.DeleteFile(ctx, path); err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("delete %s: %w", path, err))
				} else {
					e.markDeleted(target.ID(), source.ID(), path, dstState, srcState)
					result.Deleted++
					log.Printf("SyncEngine: ✗ %s: %s (deleted on %s)", source.ID(), path, target.ID())
				}
				continue
			}
			// Файл новый (или изменён после удаления на target) → копируем в target
			if err := e.transfer(ctx, source, target, srcEntry); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("transfer %s: %w", path, err))
			} else {
				e.updateState(source.ID(), target.ID(), srcEntry, srcState, dstState)
				result.Transferred++
				log.Printf("SyncEngine: ✓ %s → %s", source.ID(), path)
			}

		case !inSrc && inDst:
			if deletedOnOther(srcState, dstState, dstEntry) {
				// source удалил файл, а target его с тех пор не менял → удаляем и на target
				if err := target.DeleteFile(ctx, path); err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("delete %s: %w", path, err))
				} else {
					e.markDeleted(source.ID(), target.ID(), path, srcState, dstState)
					result.Deleted++
					log.Printf("SyncEngine: ✗ %s: %s (deleted on %s)", target.ID(), path, source.ID())
				}
				continue
			}
			// Файл новый (или изменён после удаления на source) → копируем в source
			if err := e.transfer(ctx, target, source, dstEntry); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("transfer %s: %w", path, err))
			} else {
				e.updateState(target.ID(), source.ID(), dstEntry, dstState, srcState)
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
					e.updateState(source.ID(), target.ID(), srcEntry, srcState, dstState)
					result.Transferred++
				}
				continue
			}

			// Учитываем изменения, сделанные на узлах после прошлой синхронизации
			srcState = e.observe(source.ID(), srcEntry, srcState)
			dstState = e.observe(target.ID(), dstEntry, dstState)

			rel := srcState.Clock.Compare(dstState.Clock)
			switch rel {
			case After, Equal:
				// source новее (или clock совпали, но хеши разные — редкий случай) → source → target
				if err := e.transfer(ctx, source, target, srcEntry); err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("transfer %s: %w", path, err))
				} else {
					e.updateState(source.ID(), target.ID(), srcEntry, srcState, dstState)
					result.Transferred++
				}
			case Before:
				// target новее → target → source
				if err := e.transfer(ctx, target, source, dstEntry); err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("transfer %s: %w", path, err))
				} else {
					e.updateState(target.ID(), source.ID(), dstEntry, dstState, srcState)
					result.Transferred++
				}
			case Concurrent:
				// Конфликт — параллельные изменения
				resolution, err := e.resolveConflict(ctx, source, target, srcEntry, dstEntry, srcState, dstState)
				if err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("resolve conflict %s: %w", path, err))
					continue
				}
				result.Conflicts = append(result.Conflicts, ConflictInfo{
					Path:       path,
					SourceNode: source.ID(),
					TargetNode: target.ID(),
					Resolution: resolution,
				})
				log.Printf("SyncEngine: conflict %s → resolved: %s", path, resolution)
			}
		}
	}

	log.Printf("SyncEngine: done — transferred=%d deleted=%d conflicts=%d errors=%d",
		result.Transferred, result.Deleted, len(result.Conflicts), len(result.Errors))
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

// wipeThreshold — сколько ранее синхронизированных файлов должно пропасть с опустевшего узла,
// чтобы синхронизация отказалась распространять удаление. Удаление нескольких последних
// файлов папки — обычная ситуация, исчезновение всего содержимого — признак сбоя.
const wipeThreshold = 10

// checkNotWiped возвращает ошибку, если узел не вернул ни одного файла, хотя после прошлых
// синхронизаций на нём было больше wipeThreshold файлов
func checkNotWiped(nodeID string, files []FileEntry, states map[string]*FileState) error {
	if len(files) > 0 {
		return nil
	}
	live := 0
	for _, st := range states {
		if !st.Deleted {
			live++
		}
	}
	if live <= wipeThreshold {
		return nil
	}
	return fmt.Errorf("node %s returned no files, but %d files were synced to it before — "+
		"refusing to propagate mass deletion (check that its folder is mounted; "+
		"if the files were removed on purpose, re-register the node)", nodeID, live)
}

func indexStates(states []FileState) map[string]*FileState {
	m := make(map[string]*FileState, len(states))
	for i := range states {
		m[states[i].Path] = &states[i]
	}
	return m
}

// isCurrent сообщает, что сохранённое состояние соответствует файлу на узле
func isCurrent(state *FileState, entry FileEntry) bool {
	return state != nil && !state.Deleted && state.Hash == entry.Hash
}

// deletedOnOther сообщает, что файл удалён на узле, где его сейчас нет:
// тот узел уже знал о файле (есть состояние), а на узле, где файл есть,
// он не менялся с прошлой синхронизации. Иначе файл считается новым или
// изменённым после удаления — и копируется обратно.
func deletedOnOther(missingState, presentState *FileState, present FileEntry) bool {
	return missingState != nil &&
		presentState != nil &&
		!presentState.Deleted &&
		presentState.Hash == present.Hash
}

// markDeleted записывает tombstone на оба узла после удаления файла на targetID
func (e *SyncEngine) markDeleted(deleterID, targetID, path string, deleterState, targetState *FileState) {
	clock := NewVectorClock()
	if deleterState != nil {
		clock = clock.Merge(deleterState.Clock)
	}
	if targetState != nil {
		clock = clock.Merge(targetState.Clock)
	}
	clock = clock.Increment(deleterID)

	now := time.Now()
	for _, nodeID := range []string{deleterID, targetID} {
		e.store.Set(nodeID, path, FileState{
			Path:      path,
			Clock:     clock,
			UpdatedAt: now,
			NodeID:    nodeID,
			Deleted:   true,
		})
	}
}

// ForgetNode удаляет сохранённое состояние узла. Вызывается при удалении узла и при
// смене его папки — иначе отсутствующие в новой папке файлы были бы приняты за удалённые.
func (e *SyncEngine) ForgetNode(nodeID string) {
	e.store.DeleteNode(nodeID)
}

// observe возвращает состояние файла с учётом изменений на узле:
// если текущий хеш отличается от сохранённого, узел изменил файл с прошлой
// синхронизации — увеличиваем его счётчик в векторных часах.
func (e *SyncEngine) observe(nodeID string, entry FileEntry, state *FileState) *FileState {
	if state.Hash == entry.Hash {
		return state
	}
	updatedAt := entry.ModTime
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	return &FileState{
		Path:      entry.Path,
		Hash:      entry.Hash,
		Size:      entry.Size,
		Clock:     state.Clock.Increment(nodeID),
		UpdatedAt: updatedAt,
		NodeID:    nodeID,
	}
}

// updateState записывает одинаковое состояние на оба узла после передачи from → to
func (e *SyncEngine) updateState(fromID, toID string, entry FileEntry, fromState, toState *FileState) {
	now := time.Now()

	var newClock VectorClock
	switch {
	case fromState != nil && toState != nil:
		newClock = fromState.Clock.Merge(toState.Clock)
	case fromState != nil:
		newClock = fromState.Clock.Clone()
	case toState != nil:
		newClock = toState.Clock.Increment(fromID)
	default:
		newClock = NewVectorClock().Increment(fromID)
	}

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

func (e *SyncEngine) resolveConflict(ctx context.Context, source, target Node, srcEntry, dstEntry FileEntry, srcState, dstState *FileState) (string, error) {
	switch e.strategy {
	case KeepNewest:
		if srcState.UpdatedAt.After(dstState.UpdatedAt) {
			if err := e.transfer(ctx, source, target, srcEntry); err != nil {
				return "", err
			}
			e.updateState(source.ID(), target.ID(), srcEntry, srcState, dstState)
			return "kept_source_newer", nil
		}
		if err := e.transfer(ctx, target, source, dstEntry); err != nil {
			return "", err
		}
		e.updateState(target.ID(), source.ID(), dstEntry, dstState, srcState)
		return "kept_target_newer", nil

	case KeepSource:
		if err := e.transfer(ctx, source, target, srcEntry); err != nil {
			return "", err
		}
		e.updateState(source.ID(), target.ID(), srcEntry, srcState, dstState)
		return "kept_source", nil

	case KeepBoth:
		// Версия source сохраняется рядом с суффиксом узла, оригинал на обоих узлах — версия target
		conflictPath := srcEntry.Path + ".conflict." + source.ID()
		r, err := source.ReadFile(ctx, srcEntry.Path)
		if err != nil {
			return "", fmt.Errorf("read: %w", err)
		}
		err = target.WriteFile(ctx, conflictPath, r)
		r.Close()
		if err != nil {
			return "", err
		}
		if err := e.transfer(ctx, target, source, dstEntry); err != nil {
			return "", err
		}
		e.updateState(target.ID(), source.ID(), dstEntry, dstState, srcState)
		return "kept_both", nil
	}
	return "unresolved", nil
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
