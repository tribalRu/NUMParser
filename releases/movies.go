package releases

import (
	"NUMParser/config"
	"NUMParser/db"
	"NUMParser/db/models"
	"NUMParser/utils"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

func GetNewMovies() {
	torrs := db.GetTorrs()
	var list []*models.TorrentDetails

	nowYear := time.Now().Year()
	for _, torr := range torrs {
		if torr.Categories == models.CatMovie && utils.Abs(torr.Year-nowYear) < 2 && torr.VideoQuality >= 200 {
			list = append(list, torr)
		}
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].CreateDate == list[j].CreateDate {
			if list[i].VideoQuality == list[j].VideoQuality {
				return list[i].AudioQuality > list[j].AudioQuality
			}
			return list[i].VideoQuality > list[j].VideoQuality
		}
		return list[i].CreateDate.After(list[j].CreateDate)
	})

	list = utils.UniqueTorrList(list)

	cfg := config.GetReleasesConfig()
	limit := 3000
	if cfg.MovieSort == "tracker" {
		limit = 1000
	}

	ents := FillTMDB("Movies", true, list, limit)
	ents = SortAndFilterMovies(ents, cfg.MovieSort, cfg.MovieMaxAgeMonths, 1000)

	log.Println("Found torrents:", len(ents))
	log.Println("All torrents:", len(list))

	save("movies_id.json", ents)
	utils.FreeOSMemGC()
}

// ShouldScanYear checks whether movies for the specified year need regeneration.
// Current and previous years are always scanned.
// Older years are skipped if the output file exists and no torrent for that year was added after file creation.
func ShouldScanYear(year int, torrs []*models.TorrentDetails) bool {
	currentYear := time.Now().Year()
	if year >= currentYear-1 {
		return true
	}

	destPath := filepath.Join(config.SaveReleasePath, "movies_id_"+strconv.Itoa(year)+".json")
	fi, err := os.Stat(destPath)
	if err != nil {
		return true
	}

	fileModTime := fi.ModTime()
	for _, t := range torrs {
		if (t.Categories == models.CatMovie || t.Categories == models.CatCartoonMovie) && t.Year == year {
			if t.CreateDate.After(fileModTime) {
				return true
			}
		}
	}
	return false
}

func GetNewMoviesYear(year int, allTorrs ...[]*models.TorrentDetails) {
	var torrs []*models.TorrentDetails
	if len(allTorrs) > 0 && len(allTorrs[0]) > 0 {
		torrs = allTorrs[0]
	} else {
		torrs = db.GetTorrs()
	}
	var list []*models.TorrentDetails

	for _, torr := range torrs {
		if (torr.Categories == models.CatMovie || torr.Categories == models.CatCartoonMovie) && torr.Year == year && torr.VideoQuality >= 100 {
			list = append(list, torr)
		}
	}

	list = utils.UniqueTorrList(list)

	ents := FillTMDB("Movies "+strconv.Itoa(year), true, list, 1000)

	ents = utils.Filter(ents, func(i int, e *models.Entity) bool {
		return e == nil || e.GetTorrent() == nil
	})
	ents = utils.Distinct(ents, func(e *models.Entity) string {
		return strconv.FormatInt(e.ID, 10)
	})

	calcRank := func(e *models.Entity) float64 {
		if e == nil || e.VoteCount <= 0 {
			return 0
		}
		return e.VoteAverage * math.Log(float64(e.VoteCount+1))
	}

	sort.Slice(ents, func(i, j int) bool {
		rankI := calcRank(ents[i])
		rankJ := calcRank(ents[j])

		if math.Abs(rankI-rankJ) < 0.1 {
			return ents[i].Popularity > ents[j].Popularity
		}

		return rankI > rankJ
	})

	log.Println("Found torrents:", len(ents))
	log.Println("All torrents:", len(list))

	save("movies_id_"+strconv.Itoa(year)+".json", ents)
}
