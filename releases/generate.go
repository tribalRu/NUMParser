package releases

import "log"

// GenerateAllReleases immediately rebuilds all standard categories from current DB
func GenerateAllReleases() {
	log.Println("[Releases] 1/6 Rebuilding Movies (movies_id.json)...")
	GetNewMovies()
	log.Println("[Releases] 2/6 Rebuilding 4K Movies (4k_id.json)...")
	GetFourKMovies()
	log.Println("[Releases] 3/6 Rebuilding TV Series (tv_id.json)...")
	GetNewTVs()
	log.Println("[Releases] 4/6 Rebuilding Cartoons (cartoons_id.json)...")
	GetNewCartoons()
	log.Println("[Releases] 5/6 Rebuilding Cartoons TV (cartoons_tv_id.json)...")
	GetNewCartoonsTV()
	log.Println("[Releases] 6/6 Rebuilding Anime (anime_id.json)...")
	GetNewAnime()
	log.Println("[Releases] All 6 standard categories rebuilt successfully!")
}
