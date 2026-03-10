package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// FileEntry — минимальное представление файла для построения дерева
type FileEntry struct {
	Path string
	Hash string // SHA-256 содержимого
	Size int64
}

// MerkleNode — узел дерева
type MerkleNode struct {
	Hash  string
	Left  *MerkleNode
	Right *MerkleNode
	Path  string // заполнено только у листьев
}

// MerkleTree строится над отсортированным списком файлов
type MerkleTree struct {
	Root    *MerkleNode
	entries []FileEntry
}

// BuildMerkleTree строит дерево из списка файлов
func BuildMerkleTree(files []FileEntry) *MerkleTree {
	if len(files) == 0 {
		return &MerkleTree{Root: &MerkleNode{Hash: emptyHash()}}
	}

	// Сортируем по пути для детерминированности
	sorted := make([]FileEntry, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Path < sorted[j].Path
	})

	leaves := make([]*MerkleNode, len(sorted))
	for i, f := range sorted {
		leaves[i] = &MerkleNode{
			Hash: hashLeaf(f),
			Path: f.Path,
		}
	}

	return &MerkleTree{
		Root:    buildLevel(leaves),
		entries: sorted,
	}
}

// RootHash возвращает корневой хеш дерева
func (t *MerkleTree) RootHash() string {
	if t.Root == nil {
		return emptyHash()
	}
	return t.Root.Hash
}

// Diff возвращает пути файлов которые отличаются между двумя деревьями
func Diff(a, b *MerkleTree) []string {
	if a.RootHash() == b.RootHash() {
		return nil
	}

	// Строим map path→hash для обоих деревьев
	aMap := make(map[string]string, len(a.entries))
	for _, e := range a.entries {
		aMap[e.Path] = e.Hash
	}
	bMap := make(map[string]string, len(b.entries))
	for _, e := range b.entries {
		bMap[e.Path] = e.Hash
	}

	seen := make(map[string]struct{})
	var diff []string

	for path, aHash := range aMap {
		seen[path] = struct{}{}
		if bHash, ok := bMap[path]; !ok || aHash != bHash {
			diff = append(diff, path)
		}
	}
	// Файлы есть в b но нет в a
	for path := range bMap {
		if _, ok := seen[path]; !ok {
			diff = append(diff, path)
		}
	}

	sort.Strings(diff)
	return diff
}

func buildLevel(nodes []*MerkleNode) *MerkleNode {
	if len(nodes) == 1 {
		return nodes[0]
	}

	var next []*MerkleNode
	for i := 0; i < len(nodes); i += 2 {
		if i+1 < len(nodes) {
			next = append(next, &MerkleNode{
				Hash:  hashPair(nodes[i].Hash, nodes[i+1].Hash),
				Left:  nodes[i],
				Right: nodes[i+1],
			})
		} else {
			// Нечётный узел — дублируем
			next = append(next, &MerkleNode{
				Hash:  hashPair(nodes[i].Hash, nodes[i].Hash),
				Left:  nodes[i],
				Right: nodes[i],
			})
		}
	}
	return buildLevel(next)
}

func hashLeaf(f FileEntry) string {
	h := sha256.Sum256([]byte(f.Path + ":" + f.Hash))
	return hex.EncodeToString(h[:])
}

func hashPair(a, b string) string {
	h := sha256.Sum256([]byte(a + b))
	return hex.EncodeToString(h[:])
}

func emptyHash() string {
	h := sha256.Sum256([]byte(""))
	return hex.EncodeToString(h[:])
}
