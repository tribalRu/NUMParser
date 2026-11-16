package config

import (
	"testing"
)

func TestConfigDefaultsAndNormalization(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Port != "38888" {
		t.Errorf("expected default port 38888, got %s", cfg.Port)
	}
	if cfg.Releases.MovieSort != "release_date" {
		t.Errorf("expected default movie_sort release_date, got %s", cfg.Releases.MovieSort)
	}
	if cfg.Releases.MovieMaxAgeMonths != 6 {
		t.Errorf("expected default movie_max_age_months 6, got %d", cfg.Releases.MovieMaxAgeMonths)
	}
	if cfg.Releases.TVSort != "last_air_date" {
		t.Errorf("expected default tv_sort last_air_date, got %s", cfg.Releases.TVSort)
	}
	if cfg.Releases.AnimeSort != "last_air_date" {
		t.Errorf("expected default anime_sort last_air_date, got %s", cfg.Releases.AnimeSort)
	}

	// Test normalization of invalid or mixed-case values
	cfg.Releases.MovieSort = "  RELEASE_DATE "
	cfg.Releases.TVSort = "invalid_mode"
	cfg.Releases.MovieMaxAgeMonths = -5
	cfg.Normalize()

	if cfg.Releases.MovieSort != "release_date" {
		t.Errorf("expected normalized movie_sort release_date, got %s", cfg.Releases.MovieSort)
	}
	if cfg.Releases.TVSort != "last_air_date" {
		t.Errorf("expected fallback tv_sort last_air_date, got %s", cfg.Releases.TVSort)
	}
	if cfg.Releases.MovieMaxAgeMonths != 0 {
		t.Errorf("expected negative max_age normalized to 0, got %d", cfg.Releases.MovieMaxAgeMonths)
	}
}
