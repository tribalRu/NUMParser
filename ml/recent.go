package ml

import (
	"NUMParser/db"
	"NUMParser/db/models"
	"fmt"
	"sort"
	"strings"
	"time"
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
