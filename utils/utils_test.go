package utils

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestAbs(t *testing.T) {
	if Abs(-5) != 5 {
		t.Errorf("expected 5, got %d", Abs(-5))
	}
	if Abs(5) != 5 {
		t.Errorf("expected 5, got %d", Abs(5))
	}
	if Abs(0) != 0 {
		t.Errorf("expected 0, got %d", Abs(0))
	}
}

func TestFilter(t *testing.T) {
	// Filter drops when predicate is true
	nums := []int{1, 2, 3, 4, 5}
	even := Filter(nums, func(i int, e int) bool {
		return e%2 != 0 // drop odds
	})
	if len(even) != 2 || even[0] != 2 || even[1] != 4 {
		t.Errorf("expected [2, 4], got %v", even)
	}
}

func TestDistinct(t *testing.T) {
	strs := []string{"apple", "banana", "apple", "cherry", "banana"}
	distinct := Distinct(strs, func(s string) string { return s })
	if len(distinct) != 3 {
		t.Fatalf("expected 3 distinct items, got %d", len(distinct))
	}
	if distinct[0] != "apple" || distinct[1] != "banana" || distinct[2] != "cherry" {
		t.Errorf("unexpected distinct order: %v", distinct)
	}
}

func TestClearStr(t *testing.T) {
	input := "Матрица: Воскрешение / The Matrix Resurrections (2021) BDRip!"
	cleared := ClearStr(input)
	// Only [0-9a-zа-яё] allowed
	for _, r := range cleared {
		valid := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'а' && r <= 'я') || r == 'ё'
		if !valid {
			t.Errorf("unexpected char in cleared string: %c", r)
		}
	}
}

func TestParallel(t *testing.T) {
	var count atomic.Int32
	PFor([]int{1, 2, 3, 4, 5}, func(i int, el int) {
		count.Add(1)
	})
	if count.Load() != 5 {
		t.Errorf("expected 5, got %d", count.Load())
	}

	count.Store(0)
	ParallelLimFor(0, 10, 3, func(i int) {
		count.Add(1)
	})
	if count.Load() != 10 {
		t.Errorf("expected 10, got %d", count.Load())
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "copy_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	src := filepath.Join(tmpDir, "src.txt")
	dst := filepath.Join(tmpDir, "dst.txt")

	if err := os.WriteFile(src, []byte("hello world"), 0644); err != nil {
		t.Fatalf("failed to write src: %v", err)
	}

	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "hello world" {
		t.Fatalf("dst content mismatch: %v, got %s", err, string(data))
	}
}
