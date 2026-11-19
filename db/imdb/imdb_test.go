package imdb

import (
	"NUMParser/config"
	"NUMParser/db/models"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestLiveImdbDB(t *testing.T) {
	defer os.Remove("config.yml")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 1. Live Ping
	if err := Ping(ctx); err != nil {
		t.Skipf("Live imdb-db server at %s not reachable, skipping live tests: %v", config.GetImdbConfig().Host, err)
		return
	}

	// 2. Live GetTitle (The Shawshank Redemption tt0111161)
	details, err := GetTitle(ctx, "tt0111161")
	if err != nil {
		t.Fatalf("failed to get tt0111161: %v", err)
	}
	if details == nil || details.AverageRating == nil {
		t.Fatalf("expected non-nil rating for tt0111161")
	}
	if *details.AverageRating < 9.0 {
		t.Errorf("expected Shawshank rating >= 9.0, got %f", *details.AverageRating)
	}

	// 3. Live Batch Query
	batch, err := GetTitlesBatch(ctx, []string{"tt0111161", "tt1375666"})
	if err != nil {
		t.Fatalf("batch query failed: %v", err)
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 titles in batch, got %d", len(batch))
	}
	if batch["tt1375666"] == nil || batch["tt1375666"].AverageRating == nil {
		t.Fatalf("expected non-nil rating for Inception tt1375666")
	}

	// 4. Live EnrichEntity
	ent := &models.Entity{
		Title:         "Начало",
		OriginalTitle: "Inception",
		Year:          "2010",
		ImdbID:        "tt1375666",
		VoteAverage:   6.5,
		VoteCount:     100,
	}
	enriched := EnrichEntity(ctx, ent)
	if !enriched {
		t.Fatalf("expected entity to be enriched")
	}
	if ent.ImdbRating < 8.0 {
		t.Errorf("expected Inception imdb rating >= 8.0, got %f", ent.ImdbRating)
	}
	if ent.VoteAverage != ent.ImdbRating {
		t.Errorf("expected VoteAverage to be updated with ImdbRating when prefer_imdb_rating=true")
	}
}

func TestMockImdbClient(t *testing.T) {
	defer os.Remove("config.yml")
	mockData := map[string]*TitleDetails{
		"tt9999999": {
			Tconst:        "tt9999999",
			PrimaryTitle:  "Test Movie",
			StartYear:     intPtr(2022),
			AverageRating: floatPtr(7.8),
			NumVotes:      intPtr(15000),
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/api/titles/tt9999999":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockData["tt9999999"])
		case r.URL.Path == "/api/titles":
			w.Header().Set("Content-Type", "application/json")
			tconst := r.URL.Query().Get("tconst")
			if item, ok := mockData[tconst]; ok {
				json.NewEncoder(w).Encode([]*TitleDetails{item})
			} else {
				json.NewEncoder(w).Encode([]*TitleDetails{})
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	origCfg := config.GetConfig()
	newCfg := origCfg
	newCfg.Imdb.Enabled = true
	newCfg.Imdb.Host = ts.URL
	newCfg.Imdb.TimeoutSec = 2
	newCfg.Imdb.PreferImdbRating = true
	config.SaveConfig(newCfg)
	defer config.SaveConfig(origCfg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := Ping(ctx); err != nil {
		t.Fatalf("mock ping failed: %v", err)
	}

	details, err := GetTitle(ctx, "tt9999999")
	if err != nil {
		t.Fatalf("failed to get mock title: %v", err)
	}
	if details == nil || details.AverageRating == nil || *details.AverageRating != 7.8 {
		t.Fatalf("unexpected mock rating: %+v", details)
	}

	batch, err := GetTitlesBatch(ctx, []string{"tt9999999"})
	if err != nil {
		t.Fatalf("failed mock batch: %v", err)
	}
	if batch["tt9999999"] == nil || *batch["tt9999999"].AverageRating != 7.8 {
		t.Fatalf("unexpected mock batch item: %+v", batch)
	}
}

func intPtr(i int) *int          { return &i }
func floatPtr(f float64) *float64 { return &f }
