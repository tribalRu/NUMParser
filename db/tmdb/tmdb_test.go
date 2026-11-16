package tmdb

import (
	"NUMParser/db/models"
	"NUMParser/db/storage"
	"os"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestTMDBStorageAndIndex(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "tmdb_test_*.db")
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

	movie := &models.Entity{
		ID:        1001,
		MediaType: "movie",
		Title:     "Inception",
		ImdbID:    "tt1375666",
	}

	tv := &models.Entity{
		ID:        2002,
		MediaType: "tv",
		Title:     "Breaking Bad",
		ImdbID:    "tt0903747",
	}

	AddTMDB(movie)
	AddTMDB(tv)

	// Test GetMovie
	loadedMovie := GetMovie(1001)
	if loadedMovie == nil || loadedMovie.Title != "Inception" {
		t.Fatalf("failed to get movie: %+v", loadedMovie)
	}

	// Test GetTV
	loadedTV := GetTV(2002)
	if loadedTV == nil || loadedTV.Title != "Breaking Bad" {
		t.Fatalf("failed to get tv: %+v", loadedTV)
	}

	// Test FindIMDB fast index
	foundMovie := FindIMDB("tt1375666")
	if foundMovie == nil || foundMovie.ID != 1001 {
		t.Fatalf("failed to find movie by IMDB ID: %+v", foundMovie)
	}

	foundTV := FindIMDB("tt0903747")
	if foundTV == nil || foundTV.ID != 2002 {
		t.Fatalf("failed to find tv by IMDB ID: %+v", foundTV)
	}

	// Test SetIndex and GetIndex
	torr := &models.TorrentDetails{
		Hash: "abcdef1234567890",
	}
	SetIndex(torr, movie)

	idx := GetIndex("abcdef1234567890")
	if idx != 1001 {
		t.Fatalf("expected index 1001, got %d", idx)
	}

	// Test Negative Cache (NotFound)
	unknownHash := "unknown_hash_9999"
	if IsNotFound(unknownHash) {
		t.Fatalf("fresh hash should not be in notFound cache")
	}

	SetNotFound(unknownHash)
	if !IsNotFound(unknownHash) {
		t.Fatalf("hash should be reported as notFound after SetNotFound")
	}

	// Once the movie is found and SetIndex is called, it should be removed from notFound
	SetIndex(&models.TorrentDetails{Hash: unknownHash}, movie)
	if IsNotFound(unknownHash) {
		t.Fatalf("hash should no longer be notFound after SetIndex")
	}
	if GetIndex(unknownHash) != 1001 {
		t.Fatalf("expected index 1001, got %d", GetIndex(unknownHash))
	}
}

func TestEntityCacheIsolation(t *testing.T) {
	// Verify that mutating an entity returned from cache does not mutate the cached version
	m1 := GetMovie(1001)
	if m1 == nil {
		return
	}
	t1 := &models.TorrentDetails{Title: "Torr 1"}
	m1.SetTorrent(t1)

	m2 := GetMovie(1001)
	if m2.GetTorrent() != nil {
		t.Fatalf("m2 should not inherit torrent from m1 because cache must return a copy")
	}
}
