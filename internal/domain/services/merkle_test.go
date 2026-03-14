package services

import (
	"testing"
)

func TestMerkleTree_RootHash_Empty(t *testing.T) {
	tree := BuildMerkleTree(nil)
	if tree.RootHash() != "" {
		t.Errorf("empty tree should have empty root hash")
	}
}

func TestMerkleTree_RootHash_Deterministic(t *testing.T) {
	files := map[string]string{
		"/a/b.txt": "hash1",
		"/a/c.txt": "hash2",
		"/d.txt":   "hash3",
	}

	tree1 := BuildMerkleTree(files)
	tree2 := BuildMerkleTree(files)

	if tree1.RootHash() != tree2.RootHash() {
		t.Errorf("same input should produce same root hash")
	}
}

func TestMerkleTree_RootHash_ChangesOnModification(t *testing.T) {
	files := map[string]string{"/a.txt": "hash1"}
	modified := map[string]string{"/a.txt": "hash2"}

	tree1 := BuildMerkleTree(files)
	tree2 := BuildMerkleTree(modified)

	if tree1.RootHash() == tree2.RootHash() {
		t.Errorf("different content should produce different root hash")
	}
}

func TestMerkleTree_Diff_NoDifference(t *testing.T) {
	files := map[string]string{
		"/a.txt": "hash1",
		"/b.txt": "hash2",
	}

	tree1 := BuildMerkleTree(files)
	tree2 := BuildMerkleTree(files)

	diff := tree1.Diff(tree2)
	if len(diff) != 0 {
		t.Errorf("identical trees should have no diff, got %v", diff)
	}
}

func TestMerkleTree_Diff_ModifiedFile(t *testing.T) {
	a := map[string]string{"/a.txt": "hash1", "/b.txt": "hash2"}
	b := map[string]string{"/a.txt": "hash1", "/b.txt": "hash_changed"}

	diff := BuildMerkleTree(a).Diff(BuildMerkleTree(b))

	if len(diff) != 1 || diff[0] != "/b.txt" {
		t.Errorf("expected diff [/b.txt], got %v", diff)
	}
}

func TestMerkleTree_Diff_AddedFile(t *testing.T) {
	a := map[string]string{"/a.txt": "hash1"}
	b := map[string]string{"/a.txt": "hash1", "/new.txt": "hash_new"}

	diff := BuildMerkleTree(a).Diff(BuildMerkleTree(b))

	if len(diff) != 1 || diff[0] != "/new.txt" {
		t.Errorf("expected diff [/new.txt], got %v", diff)
	}
}

func TestMerkleTree_Diff_DeletedFile(t *testing.T) {
	a := map[string]string{"/a.txt": "hash1", "/b.txt": "hash2"}
	b := map[string]string{"/a.txt": "hash1"}

	diff := BuildMerkleTree(a).Diff(BuildMerkleTree(b))

	if len(diff) != 1 || diff[0] != "/b.txt" {
		t.Errorf("expected diff [/b.txt], got %v", diff)
	}
}

func TestMerkleTree_SingleFile(t *testing.T) {
	files := map[string]string{"/only.txt": "hash1"}
	tree := BuildMerkleTree(files)

	if tree.RootHash() == "" {
		t.Errorf("single-file tree should have non-empty root hash")
	}
}
