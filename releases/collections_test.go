package releases

import (
	"NUMParser/config"
	"NUMParser/db/models"
	"NUMParser/ml"
	"testing"
	"time"
)

func TestGetCollectionIdDeduplication(t *testing.T) {
	coll := &ml.Collection{
		Title:    "Осенние загадки: Интеллектуальные головоломки",
		Overview: "Лучшие фильмы-головоломки",
		Prompt:   "Триллеры и головоломки",
	}

	// 1. Same title ("Миллионер из трущоб") with different TMDB IDs and torrent hashes
	e1 := &models.Entity{
		ID:            101,
		Title:         "Миллионер из трущоб",
		OriginalTitle: "Slumdog Millionaire",
		Year:          "2008",
		VoteAverage:   8.0,
		VoteCount:     5000,
	}
	e1.SetTorrent(&models.TorrentDetails{
		Hash:       "hash1",
		Name:       "Миллионер из трущоб / Slumdog Millionaire (2008) BDRip",
		CreateDate: time.Now(),
	})

	e2 := &models.Entity{
		ID:            102, // Different TMDB ID!
		Title:         "Миллионер из трущоб",
		OriginalTitle: "Slumdog Millionaire",
		Year:          "2008",
		VoteAverage:   8.0,
		VoteCount:     5000,
	}
	e2.SetTorrent(&models.TorrentDetails{
		Hash:       "hash2",
		Name:       "Миллионер из трущоб 1080p",
		CreateDate: time.Now(),
	})

	e3 := &models.Entity{
		ID:            103, // Another TMDB ID!
		Title:         "Миллионер из трущоб",
		OriginalTitle: "Slumdog Millionaire",
		Year:          "2008",
		VoteAverage:   8.0,
		VoteCount:     5000,
	}
	e3.SetTorrent(&models.TorrentDetails{
		Hash:       "hash3",
		Name:       "Миллионер из трущоб REMUX",
		CreateDate: time.Now(),
	})

	// 2. "Престиж" twice: one in Russian title, one matching original title
	e4 := &models.Entity{
		ID:            201,
		Title:         "Престиж",
		OriginalTitle: "The Prestige",
		Year:          "2006",
		VoteAverage:   8.5,
		VoteCount:     10000,
	}
	e4.SetTorrent(&models.TorrentDetails{
		Hash:       "hash4",
		Name:       "Престиж / The Prestige (2006) BDRip",
		CreateDate: time.Now(),
	})

	e5 := &models.Entity{
		ID:            202,
		Title:         "Престиж",
		OriginalTitle: "The Prestige",
		Year:          "2006",
		VoteAverage:   8.5,
		VoteCount:     10000,
	}
	e5.SetTorrent(&models.TorrentDetails{
		Hash:       "hash5",
		Name:       "The Prestige 2006 BDRip",
		CreateDate: time.Now(),
	})

	// 3. Unique third film
	e6 := &models.Entity{
		ID:            301,
		Title:         "Начало",
		OriginalTitle: "Inception",
		Year:          "2010",
		VoteAverage:   8.8,
		VoteCount:     20000,
	}
	e6.SetTorrent(&models.TorrentDetails{
		Hash:       "hash6",
		Name:       "Начало / Inception (2010) BDRip",
		CreateDate: time.Now(),
	})

	ents := []*models.Entity{e1, e2, e3, e4, e5, e6}

	collId := getCollectionId(coll, ents)
	if collId == nil {
		t.Fatal("expected non-nil collId")
	}

	// Should contain exactly 3 films: "Миллионер из трущоб" (once), "Престиж" (once), "Начало" (once)
	if len(collId.Parts) != 3 {
		t.Fatalf("expected exactly 3 deduplicated parts, got %d", len(collId.Parts))
	}

	expectedIDs := map[int64]bool{
		101: false,
		201: false,
		301: false,
	}

	for _, p := range collId.Parts {
		if _, ok := expectedIDs[p.Id]; !ok {
			t.Errorf("unexpected part ID: %d", p.Id)
		}
		expectedIDs[p.Id] = true
	}

	for id, found := range expectedIDs {
		if !found {
			t.Errorf("expected film with ID %d was not found in collection parts", id)
		}
	}
}

func TestIsEntityEligibleForCollection(t *testing.T) {
	cfg := config.ReleasesConfig{
		CollsMinRating:   6.0,
		CollsMinVotes:    30,
		CollsFreshMonths: 18,
	}

	now := time.Now()

	// 1. Established film with high rating -> PASS
	eHigh := &models.Entity{
		Title:       "Хороший фильм",
		Year:        "2015",
		ReleaseDate: "2015-05-10",
		VoteAverage: 7.5,
		VoteCount:   1500,
	}
	if ok, reason := isEntityEligibleForCollection(eHigh, cfg); !ok {
		t.Errorf("expected eHigh to pass: %s", reason)
	}

	// 2. Established old film with low rating -> FAIL
	eLowOld := &models.Entity{
		Title:       "Старый плохой фильм",
		Year:        "2010",
		ReleaseDate: "2010-03-15",
		VoteAverage: 4.5,
		VoteCount:   800,
	}
	if ok, reason := isEntityEligibleForCollection(eLowOld, cfg); ok {
		t.Errorf("expected eLowOld to be rejected: %s", reason)
	}

	// 3. Fresh movie in theaters with NO ratings yet (VoteCount == 0) -> PASS (Grace period)
	eFreshUnrated := &models.Entity{
		Title:       "Свежая премьера",
		Year:        now.Format("2006"),
		ReleaseDate: now.AddDate(0, -1, 0).Format("2006-01-02"), // 1 month ago
		VoteAverage: 0.0,
		VoteCount:   0,
	}
	if ok, reason := isEntityEligibleForCollection(eFreshUnrated, cfg); !ok {
		t.Errorf("expected eFreshUnrated to pass under grace period: %s", reason)
	}

	// 4. Fresh movie with very few votes (< 30) and low early score -> PASS (Grace period)
	eFreshFewVotes := &models.Entity{
		Title:       "Новинка с 5 оценками",
		Year:        now.Format("2006"),
		ReleaseDate: now.AddDate(0, -2, 0).Format("2006-01-02"),
		VoteAverage: 5.1,
		VoteCount:   5,
	}
	if ok, reason := isEntityEligibleForCollection(eFreshFewVotes, cfg); !ok {
		t.Errorf("expected eFreshFewVotes to pass under low-votes grace: %s", reason)
	}

	// 5. Fresh movie with MANY votes (>= 30) confirming low score -> FAIL
	eFreshBadConfirmed := &models.Entity{
		Title:       "Новинка с 500 плохими оценками",
		Year:        now.Format("2006"),
		ReleaseDate: now.AddDate(0, -2, 0).Format("2006-01-02"),
		VoteAverage: 4.1,
		VoteCount:   500,
	}
	if ok, reason := isEntityEligibleForCollection(eFreshBadConfirmed, cfg); ok {
		t.Errorf("expected eFreshBadConfirmed to be rejected: %s", reason)
	}

	// 6. IMDb rating takes precedence over TMDB rating
	eImdbOverride := &models.Entity{
		Title:       "Фильм с высоким IMDb",
		Year:        "2018",
		ReleaseDate: "2018-05-10",
		VoteAverage: 5.2, // TMDB score is low
		VoteCount:   100,
		ImdbRating:  7.8, // IMDb score is high!
		ImdbVotes:   25000,
	}
	if ok, reason := isEntityEligibleForCollection(eImdbOverride, cfg); !ok {
		t.Errorf("expected eImdbOverride to pass using IMDb rating: %s", reason)
	}

	// 7. Filter disabled (0.0) -> everything passes
	cfgDisabled := config.ReleasesConfig{CollsMinRating: 0.0}
	if ok, _ := isEntityEligibleForCollection(eLowOld, cfgDisabled); !ok {
		t.Errorf("expected eLowOld to pass when filter disabled")
	}
}

