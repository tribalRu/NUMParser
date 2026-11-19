package releases

import (
	"NUMParser/config"
	"NUMParser/db/models"
	"testing"
	"time"
)

func TestIsEntityEligibleGlobal(t *testing.T) {
	cfg := config.ReleasesConfig{
		MinRating:   6.0,
		MinVotes:    30,
		FreshMonths: 18,
	}

	now := time.Now()

	// 1. Movie: high rating -> PASS
	mGood := &models.Entity{
		Title:       "Interstellar",
		MediaType:   "movie",
		ReleaseDate: "2014-11-07",
		VoteAverage: 8.7,
		VoteCount:   35000,
	}
	if ok, reason := IsEntityEligible(mGood, cfg); !ok {
		t.Errorf("expected mGood to pass: %s", reason)
	}

	// 2. Movie: old low rating -> FAIL
	mBad := &models.Entity{
		Title:       "Movie 43",
		MediaType:   "movie",
		ReleaseDate: "2013-01-25",
		VoteAverage: 4.3,
		VoteCount:   2500,
	}
	if ok, reason := IsEntityEligible(mBad, cfg); ok {
		t.Errorf("expected mBad to be rejected: %s", reason)
	}

	// 3. TV Series: fresh episode (LastAirDate 2 weeks ago), 0 votes -> PASS (grace)
	tvFresh := &models.Entity{
		Title:        "New TV Show",
		MediaType:    "tv",
		FirstAirDate: now.AddDate(0, -1, 0).Format("02.01.2006"),
		LastAirDate:  now.AddDate(0, 0, -14).Format("02.01.2006"),
		VoteAverage:  0,
		VoteCount:    0,
	}
	if ok, reason := IsEntityEligible(tvFresh, cfg); !ok {
		t.Errorf("expected tvFresh to pass: %s", reason)
	}

	// 4. TV Series: old ended series with low rating -> FAIL
	tvOldBad := &models.Entity{
		Title:        "Bad Old Sitcom",
		MediaType:    "tv",
		FirstAirDate: "10.09.2010",
		LastAirDate:  "15.05.2012",
		VoteAverage:  4.8,
		VoteCount:    450,
	}
	if ok, reason := IsEntityEligible(tvOldBad, cfg); ok {
		t.Errorf("expected tvOldBad to be rejected: %s", reason)
	}

	// 5. Anime: high rating -> PASS
	animeGood := &models.Entity{
		Title:        "Attack on Titan",
		MediaType:    "tv",
		FirstAirDate: "07.04.2013",
		LastAirDate:  "04.11.2023",
		VoteAverage:  8.9,
		VoteCount:    18000,
	}
	if ok, reason := IsEntityEligible(animeGood, cfg); !ok {
		t.Errorf("expected animeGood to pass: %s", reason)
	}

	// 6. IMDb override: TMDB is low (5.4) but IMDb is high (7.6) -> PASS
	imdbWin := &models.Entity{
		Title:       "Underrated Gem",
		MediaType:   "movie",
		ReleaseDate: "2019-06-12",
		VoteAverage: 5.4,
		VoteCount:   200,
		ImdbRating:  7.6,
		ImdbVotes:   45000,
	}
	if ok, reason := IsEntityEligible(imdbWin, cfg); !ok {
		t.Errorf("expected imdbWin to pass: %s", reason)
	}

	// 7. Global filter disabled (0.0) -> PASS
	cfgZero := config.ReleasesConfig{MinRating: 0.0}
	if ok, _ := IsEntityEligible(mBad, cfgZero); !ok {
		t.Errorf("expected mBad to pass when MinRating == 0")
	}
}
