package releases

import (
	"NUMParser/db/models"
	"testing"
	"time"
)

func TestParseReleaseDate(t *testing.T) {
	cases := []struct {
		input    string
		expected string
		valid    bool
	}{
		{"15.05.2024", "2024-05-15", true},
		{"2024-05-15", "2024-05-15", true},
		{"15/05/2024", "2024-05-15", true},
		{"2024", "2024-01-01", true},
		{"", "", false},
		{"invalid", "", false},
	}

	for _, c := range cases {
		parsed, ok := ParseReleaseDate(c.input)
		if ok != c.valid {
			t.Errorf("input %q: expected valid=%v, got %v", c.input, c.valid, ok)
		}
		if ok && parsed.Format("2006-01-02") != c.expected {
			t.Errorf("input %q: expected date %q, got %q", c.input, c.expected, parsed.Format("2006-01-02"))
		}
	}
}

func TestSortAndFilterMovies(t *testing.T) {
	now := time.Now()

	// Movie 1: Fresh 2024 release, uploaded 10 days ago
	ent1 := &models.Entity{
		ID:          1,
		Title:       "Fresh Release",
		ReleaseDate: now.AddDate(0, -1, 0).Format("02.01.2006"), // 1 month ago
	}
	torr1 := &models.TorrentDetails{
		Title:        "Fresh Release 1080p",
		CreateDate:   now.AddDate(0, 0, -10),
		VideoQuality: 200,
	}
	ent1.SetTorrent(torr1)

	// Movie 2: Old 2022 release, re-uploaded today
	ent2 := &models.Entity{
		ID:          2,
		Title:       "Old Re-upload",
		ReleaseDate: now.AddDate(-2, 0, 0).Format("02.01.2006"), // 2 years ago
	}
	torr2 := &models.TorrentDetails{
		Title:        "Old Re-upload Remux",
		CreateDate:   now,
		VideoQuality: 300,
	}
	ent2.SetTorrent(torr2)

	// Movie 3: Recent release (2 months ago), uploaded 5 days ago
	ent3 := &models.Entity{
		ID:          3,
		Title:       "Recent Movie",
		ReleaseDate: now.AddDate(0, -2, 0).Format("02.01.2006"), // 2 months ago
	}
	torr3 := &models.TorrentDetails{
		Title:        "Recent Movie 1080p",
		CreateDate:   now.AddDate(0, 0, -5),
		VideoQuality: 200,
	}
	ent3.SetTorrent(torr3)

	ents := []*models.Entity{ent1, ent2, ent3}

	// Test 1: Release date mode with max age 6 months
	resRelease := SortAndFilterMovies(ents, "release_date", 6, 100)
	if len(resRelease) != 2 {
		t.Fatalf("expected 2 movies (old movie filtered out), got %d", len(resRelease))
	}
	if resRelease[0].ID != 1 {
		t.Errorf("expected top movie to be Fresh Release (ID=1), got ID=%d", resRelease[0].ID)
	}
	if resRelease[1].ID != 3 {
		t.Errorf("expected second movie to be Recent Movie (ID=3), got ID=%d", resRelease[1].ID)
	}

	// Test 2: Tracker mode (old behavior)
	resTracker := SortAndFilterMovies(ents, "tracker", 0, 100)
	if len(resTracker) != 3 {
		t.Fatalf("expected 3 movies in tracker mode, got %d", len(resTracker))
	}
	if resTracker[0].ID != 2 {
		t.Errorf("expected top movie in tracker mode to be Old Re-upload (ID=2), got ID=%d", resTracker[0].ID)
	}
}

func TestSortAndFilterTV(t *testing.T) {
	now := time.Now()

	// Series 1: Long-running show, fresh episode released 3 days ago, torrent uploaded 2 days ago
	ent1 := &models.Entity{
		ID:           101,
		Title:        "The Boys New Season",
		FirstAirDate: "15.05.2019",
		LastAirDate:  now.AddDate(0, 0, -3).Format("02.01.2006"),
	}
	torr1 := &models.TorrentDetails{
		Title:      "The Boys S04E08",
		CreateDate: now.AddDate(0, 0, -2),
	}
	ent1.SetTorrent(torr1)

	// Series 2: Old finished show (ended 2012), re-uploaded in 1080p today
	ent2 := &models.Entity{
		ID:           102,
		Title:        "Lost Complete BDRip",
		FirstAirDate: "22.09.2004",
		LastAirDate:  "23.05.2010",
	}
	torr2 := &models.TorrentDetails{
		Title:      "Lost Complete BDRip 1080p",
		CreateDate: now,
	}
	ent2.SetTorrent(torr2)

	ents := []*models.Entity{ent1, ent2}

	// Test LastAirDate mode with max age 6 months
	resTV := SortAndFilterTV(ents, "last_air_date", 6, 100)
	if len(resTV) != 1 {
		t.Fatalf("expected 1 series (Lost filtered out due to last air date in 2010), got %d", len(resTV))
	}
	if resTV[0].ID != 101 {
		t.Errorf("expected top series to be ID=101, got ID=%d", resTV[0].ID)
	}

	// Test Tracker mode
	resTracker := SortAndFilterTV(ents, "tracker", 0, 100)
	if len(resTracker) != 2 {
		t.Fatalf("expected 2 series in tracker mode, got %d", len(resTracker))
	}
	if resTracker[0].ID != 102 {
		t.Errorf("expected top series in tracker mode to be Lost (ID=102), got ID=%d", resTracker[0].ID)
	}
}
