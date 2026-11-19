package releases

import (
	"NUMParser/config"
	"NUMParser/db/imdb"
	"NUMParser/db/models"
	"NUMParser/movies/tmdb"
	"context"
	"fmt"
	"time"
)

// EnrichEntitiesBatch performs fast batch preloading of IMDb metadata for a slice of entities.
func EnrichEntitiesBatch(ents []*models.Entity) {
	if len(ents) == 0 {
		return
	}
	cfg := config.GetImdbConfig()
	if !cfg.Enabled || imdb.IsCircuitBreakerOpen() {
		return
	}
	timeout := cfg.TimeoutSec
	if timeout <= 0 {
		timeout = 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout*2)*time.Second)
	defer cancel()
	imdb.EnrichEntitiesBatch(ctx, ents)
}

// enrichWithImdb populates the entity with IMDb rating and votes from the local imdb-db server if enabled.
func enrichWithImdb(e *models.Entity) {
	if e == nil || e.ImdbRating > 0 {
		return
	}
	cfg := config.GetImdbConfig()
	if !cfg.Enabled || imdb.IsCircuitBreakerOpen() {
		return
	}
	if e.ImdbID == "" && e.GetTorrent() != nil && e.GetTorrent().IMDBID != "" {
		e.ImdbID = e.GetTorrent().IMDBID
	}
	if e.ImdbID == "" && e.ID > 0 {
		isMovie := e.MediaType != "tv" && e.MediaType != "series"
		fullEnt := tmdb.GetVideoDetails(isMovie, e.ID)
		if fullEnt != nil && fullEnt.ImdbID != "" {
			e.ImdbID = fullEnt.ImdbID
		}
	}
	timeout := cfg.TimeoutSec
	if timeout <= 0 {
		timeout = 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()
	imdb.EnrichEntity(ctx, e)
}

// GetEffectiveRating determines the rating, votes and source (IMDb or TMDB) to evaluate an entity.
func GetEffectiveRating(e *models.Entity) (float64, int, string) {
	if e == nil {
		return 0, 0, "none"
	}
	cfg := config.GetImdbConfig()
	if cfg.Enabled && cfg.PreferImdbRating && e.ImdbRating > 0 {
		return e.ImdbRating, e.ImdbVotes, "IMDb"
	}
	if e.VoteAverage > 0 || e.VoteCount > 0 {
		return e.VoteAverage, e.VoteCount, "TMDB"
	}
	if e.ImdbRating > 0 {
		return e.ImdbRating, e.ImdbVotes, "IMDb"
	}
	return 0, 0, "none"
}

// GetEntityDate extracts the release date for movies or air date for TV/Anime series.
func GetEntityDate(e *models.Entity) time.Time {
	if e == nil {
		return time.Time{}
	}
	if d := GetMovieDate(e); !d.IsZero() {
		return d
	}
	return GetTVDate(e)
}

// IsEntityEligible checks whether an entity (movie, tv series, anime, etc.) meets
// the global minimum rating threshold, while granting a grace pass for new releases.
func IsEntityEligible(e *models.Entity, relCfg config.ReleasesConfig) (bool, string) {
	if e == nil {
		return false, "nil entity"
	}

	minRating := relCfg.MinRating
	if minRating <= 0.0 {
		minRating = relCfg.CollsMinRating
	}
	if minRating <= 0.0 {
		return true, "фильтр отключен"
	}

	enrichWithImdb(e)
	rating, votes, source := GetEffectiveRating(e)
	relDate := GetEntityDate(e)
	now := time.Now()

	// 1. Проверяем, считается ли релиз свежей новинкой
	freshMonths := relCfg.FreshMonths
	if freshMonths <= 0 {
		freshMonths = relCfg.CollsFreshMonths
	}
	if freshMonths <= 0 {
		freshMonths = 18
	}
	cutoff := now.AddDate(0, -freshMonths, 0)
	isFresh := relDate.IsZero() || !relDate.Before(cutoff) // если дата не определена или позже cutoff

	// 2. Если рейтинг уже выше или равен минимальному — безоговорочно проходит
	if rating >= minRating {
		return true, fmt.Sprintf("%s %.1f >= %.1f", source, rating, minRating)
	}

	// 3. Если рейтинг ниже порога, но материал свежий:
	// А) Материал только вышел и оценок еще нет (VoteCount == 0 или rating == 0)
	if isFresh && (votes == 0 || rating == 0) {
		dateStr := "нет даты"
		if !relDate.IsZero() {
			dateStr = relDate.Format("02.01.2006")
		}
		return true, fmt.Sprintf("новинка без оценок (%s)", dateStr)
	}

	// Б) Материал вышел недавно и количество оценок статистически мало (< min_votes)
	minVotes := relCfg.MinVotes
	if minVotes <= 0 {
		minVotes = relCfg.CollsMinVotes
	}
	if minVotes <= 0 {
		minVotes = 30
	}
	if isFresh && votes < minVotes {
		return true, fmt.Sprintf("новинка с малым числом оценок (%d < %d)", votes, minVotes)
	}

	// Материал старый или имеет достаточное число оценок, подтверждающих низкий рейтинг
	return false, fmt.Sprintf("%s %.1f (%d оценок) < %.1f", source, rating, votes, minRating)
}

// isEntityEligibleForCollection is an alias for backward compatibility with collections
func isEntityEligibleForCollection(e *models.Entity, relCfg config.ReleasesConfig) (bool, string) {
	return IsEntityEligible(e, relCfg)
}
