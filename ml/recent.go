package ml

import (
	"NUMParser/db"
	"NUMParser/db/models"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

// GetRecentReleasesContext returns a formatted string of the freshest, most popular movie releases
// from the local Rutor tracker database to inject into AI prompts.
func GetRecentReleasesContext(limit int) string {
	if limit <= 0 {
		limit = 100
	}

	torrs := db.GetTorrs()
	if len(torrs) == 0 {
		return ""
	}

	currentYear := time.Now().Year()
	minYear := currentYear - 2 // Last 2-3 years

	type movieCandidate struct {
		title      string
		year       int
		seeds      int
		createDate time.Time
	}

	seen := make(map[string]bool)
	var candidates []movieCandidate

	for _, t := range torrs {
		if t.Name == "" || t.Year < minYear {
			continue
		}
		if t.Categories != models.CatMovie && t.Categories != models.CatCartoonMovie {
			continue
		}
		if t.VideoQuality < 100 { // Skip camrips/telesync
			continue
		}

		cleanKey := strings.ToLower(strings.TrimSpace(t.Name))
		if seen[cleanKey] {
			continue
		}
		seen[cleanKey] = true

		displayName := t.Name
		if len(t.Names) > 0 && t.Names[0] != "" && t.Names[0] != t.Name {
			displayName = fmt.Sprintf("%s / %s", t.Name, t.Names[0])
		}

		candidates = append(candidates, movieCandidate{
			title:      displayName,
			year:       t.Year,
			seeds:      t.Seed,
			createDate: t.CreateDate,
		})
	}

	if len(candidates) == 0 {
		return ""
	}

	// Sort by a composite score: recency (within last 90 days) and seeders
	now := time.Now()
	cutoff90Days := now.AddDate(0, -3, 0)

	sort.Slice(candidates, func(i, j int) bool {
		recencyI := candidates[i].createDate.After(cutoff90Days)
		recencyJ := candidates[j].createDate.After(cutoff90Days)

		if recencyI != recencyJ {
			return recencyI
		}
		if candidates[i].year != candidates[j].year {
			return candidates[i].year > candidates[j].year
		}
		return candidates[i].seeds > candidates[j].seeds
	})

	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	var items []string
	for _, c := range candidates {
		items = append(items, fmt.Sprintf("%s (%d)", c.title, c.year))
	}

	return strings.Join(items, "; ")
}

// GetCollectionTopicCandidates searches the local tracker database for movies matching
// the collection theme or having top seeders, ensuring AI has guaranteed matches to pick from.
func GetCollectionTopicCandidates(collTitle, collPrompt string, limit int) string {
	if limit <= 0 {
		limit = 20
	}

	torrs := db.GetTorrs()
	if len(torrs) == 0 {
		return ""
	}

	stopWords := map[string]bool{
		"фильм": true, "фильмы": true, "кино": true, "подборка": true,
		"коллекция": true, "лучшие": true, "лучших": true, "самые": true,
		"топ": true, "шедевры": true, "сезон": true, "года": true,
		"годов": true, "жанр": true, "тема": true, "про": true,
		"все": true, "для": true, "история": true, "кинематограф": true,
		"movie": true, "movies": true, "cinema": true, "best": true,
	}

	tokenize := func(text string) []string {
		var words []string
		f := func(c rune) bool {
			return !unicode.IsLetter(c) && !unicode.IsDigit(c)
		}
		for _, w := range strings.FieldsFunc(strings.ToLower(text), f) {
			if len([]rune(w)) >= 3 && !stopWords[w] {
				words = append(words, w)
			}
		}
		return words
	}

	keywords := tokenize(collTitle + " " + collPrompt)

	type matchCandidate struct {
		title string
		year  int
		score int
	}

	seen := make(map[string]bool)
	var matches []matchCandidate

	for _, t := range torrs {
		if t.Name == "" || t.Year < 1950 {
			continue
		}
		if t.Categories != models.CatMovie && t.Categories != models.CatCartoonMovie {
			continue
		}
		if t.VideoQuality < 100 {
			continue
		}

		cleanKey := strings.ToLower(strings.TrimSpace(t.Name))
		if seen[cleanKey] {
			continue
		}

		fullText := strings.ToLower(t.Name + " " + t.Title + " " + strings.Join(t.Names, " "))
		matchCount := 0
		for _, kw := range keywords {
			if strings.Contains(fullText, kw) {
				matchCount++
			}
		}

		if matchCount > 0 {
			seen[cleanKey] = true
			displayName := t.Name
			if len(t.Names) > 0 && t.Names[0] != "" && t.Names[0] != t.Name {
				displayName = fmt.Sprintf("%s / %s", t.Name, t.Names[0])
			}
			score := matchCount*1000 + t.Seed
			matches = append(matches, matchCandidate{
				title: displayName,
				year:  t.Year,
				score: score,
			})
		}
	}

	// Sort matches by relevance score
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})

	if len(matches) > limit {
		matches = matches[:limit]
	}

	if len(matches) == 0 {
		return ""
	}

	var items []string
	for _, m := range matches {
		items = append(items, fmt.Sprintf("%s (%d)", m.title, m.year))
	}

	return strings.Join(items, "; ")
}
