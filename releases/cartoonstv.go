package releases

import (
	"NUMParser/config"
	"NUMParser/db"
	"NUMParser/db/models"
	"NUMParser/utils"
	"log"
	"sort"
)

func GetNewCartoonsTV() {
	torrs := db.GetTorrs()
	var list []*models.TorrentDetails

	for _, torr := range torrs {
		if torr.Categories == models.CatCartoonSeries {
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
	if cfg.TVSort == "tracker" {
		limit = 1000
	}

	ents := FillTMDB("CartoonsTV", false, list, limit)
	ents = SortAndFilterTV(ents, cfg.TVSort, cfg.TVMaxAgeMonths, 1000)

	log.Println("Found torrents:", len(ents))
	log.Println("All torrents:", len(list))

	save("cartoons_tv_id.json", ents)
	utils.FreeOSMemGC()
}
