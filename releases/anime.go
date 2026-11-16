package releases

import (
	"NUMParser/config"
	"NUMParser/db"
	"NUMParser/db/models"
	"NUMParser/utils"
	"log"
	"sort"
)

func GetNewAnime() {
	torrs := db.GetTorrs()
	var list []*models.TorrentDetails

	for _, torr := range torrs {
		if torr.Categories == models.CatAnime {
			list = append(list, torr)
		}
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].CreateDate == list[j].CreateDate {
			return list[i].Title > list[j].Title
		}
		return list[i].CreateDate.After(list[j].CreateDate)
	})

	list = utils.UniqueTorrList(list)

	cfg := config.GetReleasesConfig()
	limit := 3000
	if cfg.AnimeSort == "tracker" {
		limit = 1000
	}

	ents := FillTMDB("Anime", false, list, limit)
	ents = SortAndFilterTV(ents, cfg.AnimeSort, cfg.AnimeMaxAgeMonths, 1000)

	log.Println("Found torrents:", len(ents))
	log.Println("All torrents:", len(list))

	save("anime_id.json", ents)
	utils.FreeOSMemGC()
}
