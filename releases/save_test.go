package releases

import (
	"NUMParser/config"
	"NUMParser/db/models"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestZipAndSave(t *testing.T) {
	now := time.Now()
	testEnt := &models.Entity{
		ID:          12345,
		Title:       "Test Movie",
		MediaType:   "movie",
		ReleaseDate: "2024-01-01",
		VoteAverage: 7.8,
		VoteCount:   100,
	}
	testTorr := &models.TorrentDetails{
		Name:         "Test Movie 1080p",
		CreateDate:   now,
		Magnet:       "magnet:?xt=urn:btih:abc",
		Size:         "2.1 GB",
		Seed:         10,
		Peer:         2,
		Link:         "/torrent/123",
		VideoQuality: models.Q_WEBDL_1080,
		AudioQuality: models.Q_D,
	}
	testEnt.SetTorrent(testTorr)

	testEnts := []*models.Entity{testEnt}

	testFileName := "unit_test_release.json"
	targetFile := filepath.Join(config.SaveReleasePath, testFileName)
	defer os.Remove(targetFile)

	save(testFileName, testEnts)

	ff, err := os.Open(targetFile)
	if err != nil {
		t.Fatalf("expected release file to exist: %v", err)
	}
	defer ff.Close()

	zr, err := gzip.NewReader(ff)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer zr.Close()

	var rid ReleasesID
	if err := json.NewDecoder(zr).Decode(&rid); err != nil {
		t.Fatalf("failed to decode gzip json: %v", err)
	}

	if len(rid.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(rid.Items))
	}
	if rid.Items[0].Id != 12345 {
		t.Errorf("expected entity ID 12345, got %d", rid.Items[0].Id)
	}
	if len(rid.Items[0].Torrent) != 1 {
		t.Fatalf("expected 1 torrent, got %d", len(rid.Items[0].Torrent))
	}
	if rid.Items[0].Torrent[0].Quality != models.Q_WEBDL_1080 {
		t.Errorf("expected quality %d, got %d", models.Q_WEBDL_1080, rid.Items[0].Torrent[0].Quality)
	}
}

func TestZipAtomic(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "atomic_zip_*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	os.Remove(tmpPath)
	defer os.Remove(tmpPath)

	rid := &ReleasesID{
		Date: "01.01.2025",
		Time: "12:00:00",
		Items: []*TmdbId{
			{Id: 999, MediaType: "movie"},
		},
	}

	if err := zip(rid, tmpPath); err != nil {
		t.Fatalf("zip failed: %v", err)
	}

	// Verify readable gzip
	f, err := os.Open(tmpPath)
	if err != nil {
		t.Fatalf("failed to open zipped file: %v", err)
	}
	defer f.Close()

	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("invalid gzip file: %v", err)
	}
	defer zr.Close()

	var loaded ReleasesID
	if err := json.NewDecoder(zr).Decode(&loaded); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if len(loaded.Items) != 1 || loaded.Items[0].Id != 999 {
		t.Errorf("unexpected loaded data: %+v", loaded)
	}
}

func TestShouldScanYear(t *testing.T) {
	currentYear := time.Now().Year()

	// 1. Current year always scanned
	if !ShouldScanYear(currentYear, nil) {
		t.Errorf("expected current year %d to always be scanned", currentYear)
	}
	// 2. Previous year always scanned
	if !ShouldScanYear(currentYear-1, nil) {
		t.Errorf("expected previous year %d to always be scanned", currentYear-1)
	}

	// 3. Old year (e.g. 1985) when file does NOT exist
	oldYear := 1985
	oldYearFile := filepath.Join(config.SaveReleasePath, "movies_id_1985.json")
	_ = os.Remove(oldYearFile)
	if !ShouldScanYear(oldYear, nil) {
		t.Errorf("expected old year %d to be scanned when file does not exist", oldYear)
	}

	// 4. Create dummy file for 1985
	_ = os.MkdirAll(config.SaveReleasePath, 0755)
	if err := os.WriteFile(oldYearFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to write dummy file: %v", err)
	}
	defer os.Remove(oldYearFile)

	// Set file mod time to 1 hour ago
	fileTime := time.Now().Add(-1 * time.Hour)
	_ = os.Chtimes(oldYearFile, fileTime, fileTime)

	// Torrents created 2 hours ago (before file) -> should NOT scan
	oldTorrs := []*models.TorrentDetails{
		{
			Categories: models.CatMovie,
			Year:       oldYear,
			CreateDate: fileTime.Add(-10 * time.Minute),
		},
	}
	if ShouldScanYear(oldYear, oldTorrs) {
		t.Errorf("expected year %d to be skipped when no new torrents exist", oldYear)
	}

	// New torrent created AFTER file mod time -> SHOULD scan
	newTorrs := []*models.TorrentDetails{
		{
			Categories: models.CatMovie,
			Year:       oldYear,
			CreateDate: fileTime.Add(10 * time.Minute),
		},
	}
	if !ShouldScanYear(oldYear, newTorrs) {
		t.Errorf("expected year %d to be scanned when newer torrent exists", oldYear)
	}
}
