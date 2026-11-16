package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCalcTime(t *testing.T) {
	tm := calcTime()
	if tm == nil {
		t.Fatalf("expected non-nil time from calcTime")
	}

	validHours := map[int]bool{
		2:  true,
		5:  true,
		8:  true,
		11: true,
		14: true,
		17: true,
		20: true,
		23: true,
	}

	if !validHours[tm.Hour()] {
		t.Errorf("expected hour to be one of [2, 5, 8, 11, 14, 17, 20, 23], got %d", tm.Hour())
	}

	// Must be in the future relative to beginning of today
	todayStart := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, time.Local)
	if tm.Before(todayStart) {
		t.Errorf("expected tm to be after or at start of today, got %v", tm)
	}
}

func TestFindScript(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "script_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ext := ".sh"
	if runtime.GOOS == "windows" {
		ext = ".bat"
	}
	dummyScript := filepath.Join(tmpDir, "test_dummy"+ext)
	if err := os.WriteFile(dummyScript, []byte("echo ok"), 0755); err != nil {
		t.Fatalf("failed to write dummy script: %v", err)
	}

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	found := findScript("test_dummy")
	if found == "" {
		t.Errorf("expected to find test_dummy%s in current working directory", ext)
	}

	notFound := findScript("non_existent_script_xyz")
	if notFound != "" {
		t.Errorf("expected empty string for non-existent script, got %s", notFound)
	}
}
