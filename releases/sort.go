package releases

import (
	"NUMParser/db/models"
	"sort"
	"strconv"
	"strings"
	"time"
)

var dateLayouts = []string{
	"02.01.2006",
	"2006-01-02",
	"02/01/2006",
	"02-01-2006",
	"2006.01.02",
}

// ParseReleaseDate attempts to parse date strings in formats commonly used by TMDB and trackers.
func ParseReleaseDate(dateStr string) (time.Time, bool) {
	dateStr = strings.TrimSpace(dateStr)
	if dateStr == "" {
		return time.Time{}, false
	}

	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, dateStr); err == nil {
			return t, true
		}
	}

	// 4-digit year fallback: e.g. "2024" -> Jan 1, 2024
	if len(dateStr) == 4 {
		if y, err := strconv.Atoi(dateStr); err == nil && y > 1800 && y < 2200 {
			return time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC), true
		}
	}

	return time.Time{}, false
}

// GetMovieDate extracts the best estimated release date for a movie entity.
func GetMovieDate(e *models.Entity) time.Time {
	if e == nil {
		return time.Time{}
	}

	if t, ok := ParseReleaseDate(e.ReleaseDate); ok {
		return t
	}
	if t, ok := ParseReleaseDate(e.Year); ok {
		return t
	}

	torr := e.GetTorrent()
	if torr != nil {
		if torr.Year > 1800 && torr.Year < 2200 {
			return time.Date(torr.Year, 1, 1, 0, 0, 0, 0, time.UTC)
		}
		if !torr.CreateDate.IsZero() {
			return torr.CreateDate
		}
	}

	return time.Time{}
}

// GetTVDate extracts the best air date for a TV/Anime series.
// By default, it prioritizes LastAirDate (fresh episode air date), falling back to FirstAirDate or ReleaseDate.
func GetTVDate(e *models.Entity) time.Time {
	if e == nil {
		return time.Time{}
	}

	if t, ok := ParseReleaseDate(e.LastAirDate); ok {
		return t
	}
	if t, ok := ParseReleaseDate(e.FirstAirDate); ok {
		return t
	}
	if t, ok := ParseReleaseDate(e.ReleaseDate); ok {
		return t
	}
	if t, ok := ParseReleaseDate(e.Year); ok {
		return t
	}

	torr := e.GetTorrent()
	if torr != nil {
		if torr.Year > 1800 && torr.Year < 2200 {
			return time.Date(torr.Year, 1, 1, 0, 0, 0, 0, time.UTC)
		}
		if !torr.CreateDate.IsZero() {
			return torr.CreateDate
		}
	}

	return time.Time{}
}

// isWithinAgeLimit verifies if a date falls within maxAgeMonths and is not unreasonably far in the future.
func isWithinAgeLimit(d time.Time, maxAgeMonths int, now time.Time) bool {
	if d.IsZero() {
		return true // Allow items without precise date if they passed category/year candidate filter
	}

	// Filter out far-future fake releases/announcements (> 14 days ahead)
	maxFuture := now.AddDate(0, 0, 14)
	if d.After(maxFuture) {
		return false
	}

	if maxAgeMonths <= 0 {
		return true
	}

	cutoff := now.AddDate(0, -maxAgeMonths, 0)
	return !d.Before(cutoff)
}

// SortAndFilterMovies filters and sorts movie entities based on release date or tracker upload date.
func SortAndFilterMovies(ents []*models.Entity, sortMode string, maxAgeMonths int, limit int) []*models.Entity {
	now := time.Now()
	var filtered []*models.Entity

	for _, e := range ents {
		if e == nil || e.GetTorrent() == nil {
			continue
		}

		if sortMode == "release_date" {
			d := GetMovieDate(e)
			if !isWithinAgeLimit(d, maxAgeMonths, now) {
				continue
			}
		}

		filtered = append(filtered, e)
	}

	if sortMode == "release_date" {
		sort.Slice(filtered, func(i, j int) bool {
			dateI := GetMovieDate(filtered[i])
			dateJ := GetMovieDate(filtered[j])

			if !dateI.Equal(dateJ) {
				return dateI.After(dateJ)
			}

			tI := filtered[i].GetTorrent()
			tJ := filtered[j].GetTorrent()
			if tI != nil && tJ != nil {
				if tI.VideoQuality != tJ.VideoQuality {
					return tI.VideoQuality > tJ.VideoQuality
				}
				if tI.AudioQuality != tJ.AudioQuality {
					return tI.AudioQuality > tJ.AudioQuality
				}
				return tI.CreateDate.After(tJ.CreateDate)
			}
			return false
		})
	} else {
		// "tracker" sort mode: order strictly by torrent upload date
		sort.Slice(filtered, func(i, j int) bool {
			tI := filtered[i].GetTorrent()
			tJ := filtered[j].GetTorrent()
			if tI != nil && tJ != nil {
				if tI.CreateDate.Equal(tJ.CreateDate) {
					if tI.VideoQuality != tJ.VideoQuality {
						return tI.VideoQuality > tJ.VideoQuality
					}
					return tI.AudioQuality > tJ.AudioQuality
				}
				return tI.CreateDate.After(tJ.CreateDate)
			}
			return false
		})
	}

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return filtered
}

// SortAndFilterTV filters and sorts TV/Anime entities based on last air date or tracker upload date.
func SortAndFilterTV(ents []*models.Entity, sortMode string, maxAgeMonths int, limit int) []*models.Entity {
	now := time.Now()
	var filtered []*models.Entity

	for _, e := range ents {
		if e == nil || e.GetTorrent() == nil {
			continue
		}

		if sortMode == "last_air_date" {
			d := GetTVDate(e)
			if !isWithinAgeLimit(d, maxAgeMonths, now) {
				continue
			}
		}

		filtered = append(filtered, e)
	}

	if sortMode == "last_air_date" {
		sort.Slice(filtered, func(i, j int) bool {
			dateI := GetTVDate(filtered[i])
			dateJ := GetTVDate(filtered[j])

			if !dateI.Equal(dateJ) {
				return dateI.After(dateJ)
			}

			tI := filtered[i].GetTorrent()
			tJ := filtered[j].GetTorrent()
			if tI != nil && tJ != nil {
				if !tI.CreateDate.Equal(tJ.CreateDate) {
					return tI.CreateDate.After(tJ.CreateDate)
				}
				return tI.Title > tJ.Title
			}
			return false
		})
	} else {
		// "tracker" sort mode
		sort.Slice(filtered, func(i, j int) bool {
			tI := filtered[i].GetTorrent()
			tJ := filtered[j].GetTorrent()
			if tI != nil && tJ != nil {
				if tI.CreateDate.Equal(tJ.CreateDate) {
					return tI.Title > tJ.Title
				}
				return tI.CreateDate.After(tJ.CreateDate)
			}
			return false
		})
	}

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return filtered
}
