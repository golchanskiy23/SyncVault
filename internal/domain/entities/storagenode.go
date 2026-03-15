package entities

import (
	"time"

	"syncvault/internal/domain/valueobjects"
)

type NodeStatus string

const (
	NodeStatusOnline   NodeStatus = "online"
	NodeStatusOffline  NodeStatus = "offline"
	NodeStatusError    NodeStatus = "error"
	NodeStatusSyncing  NodeStatus = "syncing"
)

type NodeType string

const (
	NodeTypeLocal   NodeType = "local"
	NodeTypeCloud   NodeType = "cloud"
	NodeTypeNetwork NodeType = "network"
)

type StorageNode struct {
	id          valueobjects.StorageNodeID
	name        string
	nodeType    NodeType
	status      NodeStatus
	endpoint    string
	createdAt   time.Time
	lastSeenAt  time.Time
	capacity    int64
	usedSpace   int64
	metadata    map[string]string
}

func NewStorageNode(
	name string,
	nodeType NodeType,
	endpoint string,
	capacity int64,
) *StorageNode {
	now := time.Now()
	return &StorageNode{
		id:         valueobjects.NewStorageNodeID(),
		name:       name,
		nodeType:   nodeType,
		status:     NodeStatusOffline,
		endpoint:   endpoint,
		createdAt:  now,
		lastSeenAt: now,
		capacity:   capacity,
		usedSpace:  0,
		metadata:   make(map[string]string),
	}
}

func (n *StorageNode) ID() valueobjects.StorageNodeID {
	return n.id
}

func (n *StorageNode) Name() string {
	return n.name
}

func (n *StorageNode) NodeType() NodeType {
	return n.nodeType
}

func (n *StorageNode) Status() NodeStatus {
	return n.status
}

func (n *StorageNode) Endpoint() string {
	return n.endpoint
}

func (n *StorageNode) CreatedAt() time.Time {
	return n.createdAt
}

func (n *StorageNode) LastSeenAt() time.Time {
	return n.lastSeenAt
}

func (n *StorageNode) Capacity() int64 {
	return n.capacity
}

func (n *StorageNode) UsedSpace() int64 {
	return n.usedSpace
}

func (n *StorageNode) FreeSpace() int64 {
	return n.capacity - n.usedSpace
}

func (n *StorageNode) Metadata() map[string]string {
	return n.metadata
}

func (n *StorageNode) SetOnline() {
	n.status = NodeStatusOnline
	n.lastSeenAt = time.Now()
}

func (n *StorageNode) SetOffline() {
	n.status = NodeStatusOffline
}

func (n *StorageNode) SetError() {
	n.status = NodeStatusError
}

func (n *StorageNode) SetSyncing() {
	n.status = NodeStatusSyncing
}

func (n *StorageNode) UpdateLastSeen() {
	n.lastSeenAt = time.Now()
	if n.status == NodeStatusOffline {
		n.status = NodeStatusOnline
	}
}

func (n *StorageNode) UpdateUsedSpace(space int64) {
	n.usedSpace = space
}

func (n *StorageNode) AddUsedSpace(delta int64) {
	newUsedSpace := n.usedSpace + delta
	if newUsedSpace >= 0 && newUsedSpace <= n.capacity {
		n.usedSpace = newUsedSpace
	}
}

func (n *StorageNode) SetMetadata(key, value string) {
	if n.metadata == nil {
		n.metadata = make(map[string]string)
	}
	n.metadata[key] = value
}

func (n *StorageNode) GetMetadata(key string) string {
	if n.metadata == nil {
		return ""
	}
	return n.metadata[key]
}

func (n *StorageNode) IsOnline() bool {
	return n.status == NodeStatusOnline || n.status == NodeStatusSyncing
}

func (n *StorageNode) HasEnoughSpace(requiredSpace int64) bool {
	return n.FreeSpace() >= requiredSpace
}

func (n *StorageNode) IsLocal() bool {
	return n.nodeType == NodeTypeLocal
}

func (n *StorageNode) IsCloud() bool {
	return n.nodeType == NodeTypeCloud
}
