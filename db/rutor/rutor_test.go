package rutor

import (
	"NUMParser/db/models"
	"NUMParser/db/storage"
	"fmt"
	"os"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestRutorSaveAndLoad(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "rutor_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	db, err := bolt.Open(tmpPath, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("failed to open bolt db: %v", err)
	}
	defer db.Close()
	storage.DB = db

	// Clear state
	torrs = nil
	hashIndex = nil
	dirtyHashes = nil
	IsTorrsChange = false

	t1 := &models.TorrentDetails{
		Title: "Test Movie 1",
		Hash:  "HASH1234567890",
		Link:  "/torrent/1",
	}
	t2 := &models.TorrentDetails{
		Title: "Test Movie 2",
		Hash:  "HASH0987654321",
		Link:  "/torrent/2",
	}

	AddTorr(t1)
	AddTorr(t2)

	if len(GetTorrs()) != 2 {
		t.Fatalf("expected 2 torrents, got %d", len(GetTorrs()))
	}

	SaveTorrs()

	// Reset in-memory slice and reload from DB
	torrs = nil
	hashIndex = nil
	Init()

	loaded := GetTorrs()
	if len(loaded) != 2 {
		t.Fatalf("expected 2 torrents loaded from DB, got %d", len(loaded))
	}

	// Test incremental update
	t3 := &models.TorrentDetails{
		Title: "Test Movie 3",
		Hash:  "HASHNEW12345",
		Link:  "/torrent/3",
	}
	AddTorr(t3)
	SaveTorrs()

	torrs = nil
	hashIndex = nil
	Init()

	loaded = GetTorrs()
	if len(loaded) != 3 {
		t.Fatalf("expected 3 torrents loaded after update, got %d", len(loaded))
	}
}

func TestRutorBatchAndDedup(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "rutor_batch_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	db, err := bolt.Open(tmpPath, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		t.Fatalf("failed to open bolt db: %v", err)
	}
	defer db.Close()
	storage.DB = db

	torrs = nil
	hashIndex = nil
	dirtyHashes = nil
	IsTorrsChange = false

	// Add 5000 items to test batching (> 2000 batchSize)
	n := 5000
	for i := 0; i < n; i++ {
		AddTorr(&models.TorrentDetails{
			Title:      fmt.Sprintf("Movie %d", i),
			Hash:       fmt.Sprintf("HASH%08d", i),
			Link:       fmt.Sprintf("/torrent/%d", i),
			CreateDate: time.Now(),
		})
	}

	if len(GetTorrs()) != n {
		t.Fatalf("expected %d torrents, got %d", n, len(GetTorrs()))
	}

	t0 := time.Now()
	SaveTorrs()
	saveDuration := time.Since(t0)
	t.Logf("Saved %d items in %v", n, saveDuration)
	if saveDuration > 5*time.Second {
		t.Fatalf("SaveTorrs took too long: %v", saveDuration)
	}

	// Test deduplication of superseding link
	// Add updated version of torrent 100 with a new hash
	AddTorr(&models.TorrentDetails{
		Title:      "Movie 100 Updated",
		Hash:       "HASH_100_NEW",
		Link:       "/torrent/100",
		CreateDate: time.Now(),
	})

	SaveTorrs()

	// Reload from DB and verify the older hash was replaced
	torrs = nil
	hashIndex = nil
	Init()

	loaded := GetTorrs()
	if len(loaded) != n {
		t.Fatalf("expected %d torrents after supersede dedup, got %d", n, len(loaded))
	}

	foundUpdated := false
	for _, it := range loaded {
		if it.Link == "/torrent/100" {
			if it.Hash != "HASH_100_NEW" {
				t.Fatalf("expected hash to be HASH_100_NEW, got %s", it.Hash)
			}
			foundUpdated = true
		}
	}
	if !foundUpdated {
		t.Fatalf("updated torrent 100 not found in loaded torrents")
	}
}
