package services

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// MerkleNode is a node in the Merkle tree.
// Leaf nodes represent individual files; internal nodes aggregate their children.
type MerkleNode struct {
	Hash     []byte
	Left     *MerkleNode
	Right    *MerkleNode
	FilePath string // non-empty only for leaf nodes
}

// MerkleTree is a binary hash tree over a set of files.
// It allows efficient detection of which files differ between two nodes.
type MerkleTree struct {
	Root  *MerkleNode
	files []merkleLeaf // sorted leaf data, kept for Diff
}

type merkleLeaf struct {
	path string
	hash string
}

// BuildMerkleTree constructs a Merkle tree from a map of filePath → fileHash.
// Files are sorted by path for deterministic tree structure.
func BuildMerkleTree(files map[string]string) *MerkleTree {
	if len(files) == 0 {
		return &MerkleTree{}
	}

	// Sort leaves for determinism
	leaves := make([]merkleLeaf, 0, len(files))
	for path, hash := range files {
		leaves = append(leaves, merkleLeaf{path: path, hash: hash})
	}
	sort.Slice(leaves, func(i, j int) bool {
		return leaves[i].path < leaves[j].path
	})

	// Build leaf nodes
	nodes := make([]*MerkleNode, len(leaves))
	for i, leaf := range leaves {
		nodes[i] = &MerkleNode{
			Hash:     hashLeaf(leaf.path, leaf.hash),
			FilePath: leaf.path,
		}
	}

	root := buildTree(nodes)
	return &MerkleTree{Root: root, files: leaves}
}

// RootHash returns the hex-encoded root hash, or empty string for an empty tree.
func (t *MerkleTree) RootHash() string {
	if t.Root == nil {
		return ""
	}
	return hex.EncodeToString(t.Root.Hash)
}

// Diff returns the file paths that differ between t and other.
// A path is included if it exists in one tree but not the other,
// or if it exists in both but with different hashes.
func (t *MerkleTree) Diff(other *MerkleTree) []string {
	aMap := make(map[string]string, len(t.files))
	for _, l := range t.files {
		aMap[l.path] = l.hash
	}

	bMap := make(map[string]string, len(other.files))
	for _, l := range other.files {
		bMap[l.path] = l.hash
	}

	seen := make(map[string]struct{})
	var changed []string

	for path, hashA := range aMap {
		seen[path] = struct{}{}
		if hashB, ok := bMap[path]; !ok || hashA != hashB {
			changed = append(changed, path)
		}
	}

	for path := range bMap {
		if _, ok := seen[path]; !ok {
			changed = append(changed, path)
		}
	}

	sort.Strings(changed)
	return changed
}

// buildTree recursively combines nodes into a binary tree bottom-up.
func buildTree(nodes []*MerkleNode) *MerkleNode {
	if len(nodes) == 1 {
		return nodes[0]
	}

	var parents []*MerkleNode
	for i := 0; i < len(nodes); i += 2 {
		left := nodes[i]
		var right *MerkleNode
		if i+1 < len(nodes) {
			right = nodes[i+1]
		} else {
			// Odd number of nodes: duplicate the last one
			right = left
		}
		parent := &MerkleNode{
			Hash:  hashPair(left.Hash, right.Hash),
			Left:  left,
			Right: right,
		}
		parents = append(parents, parent)
	}

	return buildTree(parents)
}

func hashLeaf(path, fileHash string) []byte {
	h := sha256.New()
	h.Write([]byte(path))
	h.Write([]byte(fileHash))
	return h.Sum(nil)
}

func hashPair(left, right []byte) []byte {
	h := sha256.New()
	h.Write(left)
	h.Write(right)
	return h.Sum(nil)
}
