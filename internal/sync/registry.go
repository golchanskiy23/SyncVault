package sync

import (
	"fmt"
	"sync"
	"time"
)

// NodeInfo описывает зарегистрированный узел в сети
type NodeInfo struct {
	ID        string
	Type      string    // simple, google_drive, remote_simple
	Endpoint  string    // адрес агента (если сетевой узел)
	AccountID string    // для Google Drive
	LastSeen  time.Time
	Online    bool
}

// NodeRegistry хранит все известные узлы системы
type NodeRegistry struct {
	mu    sync.RWMutex
	nodes map[string]*NodeInfo
	impls map[string]Node // живые реализации
}

func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		nodes: make(map[string]*NodeInfo),
		impls: make(map[string]Node),
	}
}

// Register регистрирует узел с его реализацией (impl может быть nil для drive-узлов)
func (r *NodeRegistry) Register(info NodeInfo, impl Node) {
	r.mu.Lock()
	defer r.mu.Unlock()
	info.LastSeen = time.Now()
	info.Online = true
	r.nodes[info.ID] = &info
	if impl != nil {
		r.impls[info.ID] = impl
	}
}

// SetDriveNode устанавливает реальную реализацию для drive-узла после его регистрации
func (r *NodeRegistry) SetDriveNode(id string, impl Node) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.impls[id] = impl
}

// Unregister удаляет узел
func (r *NodeRegistry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.nodes, id)
	delete(r.impls, id)
}

// Get возвращает реализацию узла по ID
func (r *NodeRegistry) Get(id string) (Node, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	impl, ok := r.impls[id]
	if !ok {
		return nil, fmt.Errorf("node %s not found", id)
	}
	if impl == nil {
		return nil, fmt.Errorf("node %s has no implementation (drive node not yet wired)", id)
	}
	return impl, nil
}

// List возвращает все онлайн узлы
func (r *NodeRegistry) List() []NodeInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []NodeInfo
	for _, n := range r.nodes {
		if n.Online {
			result = append(result, *n)
		}
	}
	return result
}

// AllNodes возвращает все реализации онлайн узлов (только с живой impl)
func (r *NodeRegistry) AllNodes() []Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []Node
	for id, info := range r.nodes {
		if info.Online && r.impls[id] != nil {
			result = append(result, r.impls[id])
		}
	}
	return result
}

// Heartbeat обновляет время последнего контакта с узлом
func (r *NodeRegistry) Heartbeat(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n, ok := r.nodes[id]; ok {
		n.LastSeen = time.Now()
		n.Online = true
	}
}

// MarkOffline помечает узел как недоступный
func (r *NodeRegistry) MarkOffline(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n, ok := r.nodes[id]; ok {
		n.Online = false
	}
}
