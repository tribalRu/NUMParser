package releases

import (
	"NUMParser/config"
	"NUMParser/db"
	"NUMParser/db/imdb"
	"NUMParser/db/models"
	"NUMParser/ml"
	"NUMParser/movies/tmdb"
	"NUMParser/releases/cover"
	"NUMParser/utils"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	isGenerating    atomic.Bool
	tmdbSearchCache sync.Map
)

// IsGenerating returns true if collections are currently being generated
func IsGenerating() bool {
	return isGenerating.Load()
}

func GetCollections() {
	if isGenerating.Swap(true) {
		log.Println("Collections generation is already in progress, skipping duplicate request")
		return
	}
	defer isGenerating.Store(false)

	log.Println("Update collections: generating fresh weekly collections...")

	const weeklyCount = 25
	collections, err := ml.GenCollection(weeklyCount)
	if err != nil || len(collections) == 0 {
		log.Println("Error while generating collections:", err)
		return
	}

	if ml.CollsConfig == nil {
		ml.CollsConfig = &ml.CollectionsConfig{}
	}
	ml.CollsConfig.Collections = collections
	ml.CollsConfig.LastUpdated = time.Now()
	ml.SaveConfig()

	log.Printf("Weekly collections count: %d\n", len(ml.CollsConfig.Collections))

	var collsId []*CollectionId

	os.MkdirAll(filepath.Join(config.SaveReleasePath, "imgs"), 0755)

	for c, coll := range ml.CollsConfig.Collections {
		fmt.Printf("Get movies for coll [%d/%d]: %s\n", c+1, len(ml.CollsConfig.Collections), coll.Title)
		fmt.Println("Overview:", coll.Overview)
		fmt.Println("Prompt:", coll.Prompt)

		var movies []*ml.MovieInfo
		for i := 0; i < 3; i++ {
			movies, err = ml.GetCollectionMovies(coll)
			if err == nil && len(movies) > 0 {
				break
			}
			log.Printf("AI error for '%s': %v (retry %d/3)", coll.Title, err, i+1)
			time.Sleep(time.Second * 2)
		}

		if len(movies) == 0 {
			log.Printf("Skipping collection %q: no movies found", coll.Title)
			continue
		}

		relCfg := config.GetReleasesConfig()
		var ents []*models.Entity
		var found []*models.Entity

		fmt.Println("Search tmdb ids:")
		for _, movie := range movies {
			fmt.Printf("%s (%s) == ", movie.Title, movie.Year)
			e := findTmdbMlMovie(movie)
			if e != nil {
				enrichWithImdb(e)

				eligible, reason := isEntityEligibleForCollection(e, relCfg)
				if !eligible {
					fmt.Printf("%s | %s (%v): отсеян по рейтингу [%s]\n", e.Title, e.OriginalTitle, e.Year, reason)
					continue
				}

				rating, votes, source := GetEffectiveRating(e)
				fmt.Printf("%s | %s (%v): %v, %.2f [%s: %.1f (%d votes)]\n",
					e.Title, e.OriginalTitle, e.Year, e.ID, e.VoteAverage*math.Log(float64(e.VoteCount+1)), source, rating, votes)
				ents = append(ents, e)
			} else {
				fmt.Println("TMDB not found")
			}
		}

		fmt.Println("Found tmdb for coll:", coll.Title, len(ents))
		if len(ents) > 0 {
			// Множественная дедупликация сущностей TMDB:
			// Фильтруем по ID, очищенному русскому названию и английскому названию
			seenEntityID := make(map[int64]bool)
			seenEntityTitle := make(map[string]bool)
			var uniqueEnts []*models.Entity
			for _, e := range ents {
				if e == nil {
					continue
				}
				if seenEntityID[e.ID] {
					continue
				}
				cleanT := utils.ClearStr(e.Title)
				if cleanT != "" && seenEntityTitle[cleanT] {
					continue
				}
				cleanOrig := utils.ClearStr(e.OriginalTitle)
				if cleanOrig != "" && seenEntityTitle[cleanOrig] {
					continue
				}
				seenEntityID[e.ID] = true
				if cleanT != "" {
					seenEntityTitle[cleanT] = true
				}
				if cleanOrig != "" {
					seenEntityTitle[cleanOrig] = true
				}
				uniqueEnts = append(uniqueEnts, e)
			}
			ents = uniqueEnts
			fmt.Println("Search torrents for cols (unique):", len(ents))

			seenTorrHash := make(map[string]bool)
			seenTorrName := make(map[string]bool)
			for i, e := range ents {
				list := searchTorrForEntity(e)
				if len(list) > 0 {
					sort.Slice(list, func(i, j int) bool {
						if list[i].CreateDate == list[j].CreateDate {
							if list[i].VideoQuality == list[j].VideoQuality {
								return list[i].AudioQuality > list[j].AudioQuality
							}
							return list[i].VideoQuality > list[j].VideoQuality
						}
						return list[i].CreateDate.After(list[j].CreateDate)
					})

					// Ищем лучший торрент, который еще не добавлен в текущую подборку
					var bestTorr *models.TorrentDetails
					for _, torrCand := range list {
						if torrCand == nil {
							continue
						}
						if torrCand.Hash != "" && seenTorrHash[torrCand.Hash] {
							continue
						}
						cName := utils.ClearStr(torrCand.Name)
						if cName != "" && seenTorrName[cName] {
							continue
						}
						bestTorr = torrCand
						break
					}

					if bestTorr != nil {
						if bestTorr.Hash != "" {
							seenTorrHash[bestTorr.Hash] = true
						}
						cName := utils.ClearStr(bestTorr.Name)
						if cName != "" {
							seenTorrName[cName] = true
						}
						e.SetTorrent(bestTorr)
						found = append(found, e)
					}
				}
				log.Println("Fill coll:", coll.Title, i+1, "/", len(ents))
			}

			if len(found) > 0 {
				fmt.Println("Found torrents for coll:", coll.Title, len(found))
				collId := getCollectionId(coll, found)
				collsId = append(collsId, collId)

				// строим картинку
				var urls []string
				var backdrop string

				for _, e := range found {
					if e.PosterPath != "" {
						urls = append(urls, strings.ReplaceAll(e.PosterPath, "w342", "w500"))
						if backdrop == "" {
							backdrop = strings.ReplaceAll(e.BackdropPath, "w780", "w1280")
						}
					}
					if len(urls) == 4 {
						break
					}
				}

				if backdrop == "" {
					for _, e := range found {
						if e.BackdropPath != "" {
							backdrop = strings.ReplaceAll(e.BackdropPath, "w780", "w1280")
							break
						}
					}
				}

				// если постеров нет вообще — просто не вызываем BuildCover
				if len(urls) > 0 {
					imgfn := strconv.Itoa(c) + ".png"
					imgfn = filepath.Join(config.SaveReleasePath, "imgs", imgfn)

					for e := 0; e < 10; e++ {
						err = cover.BuildCover(urls, backdrop, imgfn)
						if err == nil {
							break
						}
					}
					if err != nil {
						log.Println("Cover build failed:", err)
						imgfn = ""
					}
					collId.PosterPath = strings.ReplaceAll(filepath.ToSlash(imgfn), "public", "")
					if backdrop != "" {
						collId.BackdropPath = cleanTmdbImgPath(backdrop)
					}
				}
			}
		}

		fmt.Println()
		// Polite delay between collections to avoid exceeding rate limits (8000 TPM on Groq Free)
		time.Sleep(5 * time.Second)
	}

	ml.SaveConfig()

	//Save collections

	os.MkdirAll(config.SaveReleasePath, 0777)
	fname := filepath.Join(config.SaveReleasePath, "colls_movie_ai.json")
	tempPath := fname + ".tmp"

	ff, err := os.Create(tempPath)
	if err != nil {
		log.Println("Error creating temp collections file:", err)
		return
	}

	zw := gzip.NewWriter(ff)
	err = json.NewEncoder(zw).Encode(collsId)
	if err != nil {
		zw.Close()
		ff.Close()
		_ = os.Remove(tempPath)
		log.Println("Error save collections:", err)
		return
	}

	if err := zw.Close(); err != nil {
		ff.Close()
		_ = os.Remove(tempPath)
		log.Println("Error closing gzip writer for collections:", err)
		return
	}

	if err := ff.Close(); err != nil {
		_ = os.Remove(tempPath)
		log.Println("Error closing collections file:", err)
		return
	}

	if err := os.Rename(tempPath, fname); err != nil {
		_ = os.Remove(fname)
		if err := os.Rename(tempPath, fname); err != nil {
			log.Println("Error renaming temp collections file:", err)
		}
	}

	utils.FreeOSMemGC()
}

func searchTorrForEntity(e *models.Entity) []*models.TorrentDetails {
	trySearch := func(q string) []*models.TorrentDetails {
		q = strings.TrimSpace(q)
		if q == "" {
			return nil
		}
		return db.SearchTorr(q)
	}

	cleanRu := strings.ReplaceAll(e.Title, "ё", "е")
	cleanRu = strings.ReplaceAll(cleanRu, "Ё", "Е")

	// 1. Русское название + год
	if list := trySearch(cleanRu + " " + e.Year); len(list) > 0 {
		return list
	}
	if cleanRu != e.Title {
		if list := trySearch(e.Title + " " + e.Year); len(list) > 0 {
			return list
		}
	}

	// 2. Оригинальное название + год (если отличается)
	if e.OriginalTitle != "" && e.OriginalTitle != e.Title {
		if list := trySearch(e.OriginalTitle + " " + e.Year); len(list) > 0 {
			return list
		}
	}

	// 3. Дополнительные английские / альтернативные названия + год
	for _, name := range getEnNames(e.Titles) {
		if name != "" && name != e.Title && name != e.OriginalTitle {
			if list := trySearch(name + " " + e.Year); len(list) > 0 {
				return list
			}
		}
	}

	// 4. Русское + оригинальное название + год (полная связка)
	if e.OriginalTitle != "" && e.OriginalTitle != e.Title {
		if list := trySearch(cleanRu + " " + e.OriginalTitle + " " + e.Year); len(list) > 0 {
			return list
		}
	}

	// 5. Только русское название (с фильтром по году +/- 1 год)
	if list := trySearch(cleanRu); len(list) > 0 {
		y, _ := strconv.Atoi(e.Year)
		var matched []*models.TorrentDetails
		for _, t := range list {
			if y == 0 || t.Year == 0 || utils.Abs(t.Year-y) <= 1 {
				matched = append(matched, t)
			}
		}
		if len(matched) > 0 {
			return matched
		}
	}

	// 6. Только оригинальное название (с фильтром по году +/- 1 год)
	if e.OriginalTitle != "" && e.OriginalTitle != e.Title {
		if list := trySearch(e.OriginalTitle); len(list) > 0 {
			y, _ := strconv.Atoi(e.Year)
			var matched []*models.TorrentDetails
			for _, t := range list {
				if y == 0 || t.Year == 0 || utils.Abs(t.Year-y) <= 1 {
					matched = append(matched, t)
				}
			}
			if len(matched) > 0 {
				return matched
			}
		}
	}

	return nil
}

func cleanTmdbImgPath(path string) string {
	if path == "" {
		return ""
	}
	if idx := strings.LastIndex(path, "/"); idx != -1 {
		return path[idx:]
	}
	return "/" + path
}

func enrichWithImdb(e *models.Entity) {
	if e == nil {
		return
	}
	cfg := config.GetImdbConfig()
	if !cfg.Enabled {
		return
	}
	if e.ImdbID == "" {
		fullEnt := tmdb.GetVideoDetails(true, e.ID)
		if fullEnt != nil && fullEnt.ImdbID != "" {
			e.ImdbID = fullEnt.ImdbID
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutSec)*time.Second)
	defer cancel()
	imdb.EnrichEntity(ctx, e)
}

// GetEffectiveRating returns the active rating, vote count, and data source ("imdb" or "tmdb").
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

// isEntityEligibleForCollection determines if an entity satisfies rating requirements,
// granting a grace period for newly released/in-theater movies or unrated materials.
func isEntityEligibleForCollection(e *models.Entity, relCfg config.ReleasesConfig) (bool, string) {
	if e == nil {
		return false, "nil entity"
	}

	if relCfg.CollsMinRating <= 0 {
		return true, "filter disabled"
	}

	rating, votes, source := GetEffectiveRating(e)

	// 1. Удовлетворяет порогу рейтинга
	if rating >= relCfg.CollsMinRating {
		return true, fmt.Sprintf("%s %.1f >= %.1f", source, rating, relCfg.CollsMinRating)
	}

	// 2. Проверка льготы для новинок проката и фильмов без устоявшихся оценок
	relDate := GetMovieDate(e)
	freshMonths := relCfg.CollsFreshMonths
	if freshMonths <= 0 {
		freshMonths = 18
	}
	cutoffDate := time.Now().AddDate(0, -freshMonths, 0)
	isFresh := relDate.IsZero() || relDate.After(cutoffDate)

	// А) Фильм только вышел и оценок еще нет (VoteCount == 0 или rating == 0)
	if isFresh && (votes == 0 || rating == 0) {
		dateStr := "нет даты"
		if !relDate.IsZero() {
			dateStr = relDate.Format("02.01.2006")
		}
		return true, fmt.Sprintf("новинка проката без оценок (%s)", dateStr)
	}

	// Б) Фильм вышел недавно и количество оценок статистически мало (< colls_min_votes)
	minVotes := relCfg.CollsMinVotes
	if minVotes <= 0 {
		minVotes = 30
	}
	if isFresh && votes < minVotes {
		return true, fmt.Sprintf("новинка проката с малым числом оценок (%d < %d)", votes, minVotes)
	}

	// Фильм старый или имеет достаточное число оценок, подтверждающих низкий рейтинг
	return false, fmt.Sprintf("%s %.1f (%d оценок) < %.1f", source, rating, votes, relCfg.CollsMinRating)
}

func getCollectionId(coll *ml.Collection, ents []*models.Entity) *CollectionId {
	rid := &CollectionId{
		Name:         coll.Title,
		Overview:     coll.Overview,
		Parts:        nil,
		PosterPath:   "",
		BackdropPath: "",
	}

	relCfg := config.GetReleasesConfig()
	seenID := make(map[int64]bool)
	seenHash := make(map[string]bool)
	seenTitle := make(map[string]bool)
	seenTorrName := make(map[string]bool)
	rutorHost := config.RutorHost()

	for _, e := range ents {
		if e != nil && e.GetTorrent() != nil {
			if eligible, reason := isEntityEligibleForCollection(e, relCfg); !eligible {
				log.Printf("[Collection %q] Skipping %q (%v): %s\n", coll.Title, e.Title, e.Year, reason)
				continue
			}

			d := e.GetTorrent()

			// Строгая многоуровневая дедупликация: исключаем повторение фильма по TMDB ID,
			// названию (рус/ориг), хэшу торрента и названию раздачи.
			if seenID[e.ID] {
				continue
			}
			if d.Hash != "" && seenHash[d.Hash] {
				continue
			}
			cleanT := utils.ClearStr(e.Title)
			if cleanT != "" && seenTitle[cleanT] {
				continue
			}
			cleanOrig := utils.ClearStr(e.OriginalTitle)
			if cleanOrig != "" && seenTitle[cleanOrig] {
				continue
			}
			cleanTorr := utils.ClearStr(d.Name)
			if cleanTorr != "" && seenTorrName[cleanTorr] {
				continue
			}

			seenID[e.ID] = true
			if d.Hash != "" {
				seenHash[d.Hash] = true
			}
			if cleanT != "" {
				seenTitle[cleanT] = true
			}
			if cleanOrig != "" {
				seenTitle[cleanOrig] = true
			}
			if cleanTorr != "" {
				seenTorrName[cleanTorr] = true
			}
			var countries []string
			if len(e.ProductionCountries) > 0 {
				for _, c := range e.ProductionCountries {
					countries = append(countries, c.Iso31661)
				}
			} else {
				countries = e.OriginCountry
			}
			t := Torrent{
				Name:     d.Name,
				Date:     d.CreateDate.Format("02.01.2006"),
				Magnet:   d.Magnet,
				Size:     d.Size,
				Upload:   strconv.Itoa(d.Seed),
				Download: strconv.Itoa(d.Peer),
				Source:   "Rutor",
				Link:     config.JoinRutorLink(rutorHost, d.Link),
				Quality:  d.VideoQuality,
				Voice:    d.AudioQuality,
			}
			tid := &TmdbId{
				Id:           e.ID,
				MediaType:    e.MediaType,
				Title:        e.Title,
				PosterPath:   cleanTmdbImgPath(e.PosterPath),
				BackdropPath: cleanTmdbImgPath(e.BackdropPath),
				GenreIds:     e.GenresIds,
				VoteAverage:  e.VoteAverage,
				VoteCount:    e.VoteCount,
				ImdbRating:   e.ImdbRating,
				ImdbVotes:    e.ImdbVotes,
				ImdbID:       e.ImdbID,
				Countries:    countries,
				ReleaseDate:  e.ReleaseDate,
				Torrent:      []*Torrent{&t},
			}
			rid.Parts = append(rid.Parts, tid)
		}
	}

	calcRank := func(voteAvg float64, voteCount int) float64 {
		if voteCount <= 0 {
			return 0
		}
		return voteAvg * math.Log(float64(voteCount+1))
	}

	sort.Slice(rid.Parts, func(i, j int) bool {
		return calcRank(rid.Parts[i].VoteAverage, rid.Parts[i].VoteCount) > calcRank(rid.Parts[j].VoteAverage, rid.Parts[j].VoteCount)
	})

	sort.Slice(ents, func(i, j int) bool {
		return calcRank(ents[i].VoteAverage, ents[i].VoteCount) > calcRank(ents[j].VoteAverage, ents[j].VoteCount)
	})

	return rid
}

func findTmdbMlMovie(movie *ml.MovieInfo) *models.Entity {
	if movie == nil || strings.TrimSpace(movie.Title) == "" {
		return nil
	}

	cacheKey := strings.ToLower(strings.TrimSpace(movie.Title)) + ":" + strings.TrimSpace(movie.Year)
	if val, ok := tmdbSearchCache.Load(cacheKey); ok {
		if val == nil {
			return nil
		}
		return val.(*models.Entity)
	}

	saveAndReturn := func(res *models.Entity) *models.Entity {
		tmdbSearchCache.Store(cacheKey, res)
		return res
	}

	yearml, _ := strconv.Atoi(movie.Year)

	searchTMDB := func(query string) []*models.Entity {
		query = strings.TrimSpace(query)
		if query == "" {
			return nil
		}
		res := tmdb.Search(true, query)
		if len(res) == 0 {
			cleanRu := strings.ReplaceAll(query, "ё", "е")
			cleanRu = strings.ReplaceAll(cleanRu, "Ё", "Е")
			if cleanRu != query {
				res = tmdb.Search(true, cleanRu)
			}
		}
		return res
	}

	rawTitle := movie.Title
	list := searchTMDB(rawTitle)
	if len(list) == 0 && strings.Contains(rawTitle, "/") {
		parts := strings.Split(rawTitle, "/")
		for _, p := range parts {
			if list = searchTMDB(p); len(list) > 0 {
				break
			}
		}
	}

	matchesYear := func(e *models.Entity) bool {
		y := tmdb.GetEntityYear(e)
		if y == 0 || yearml == 0 {
			return true
		}
		return utils.Abs(y-yearml) <= 1
	}

	list = utils.Filter(list, func(i int, e *models.Entity) bool {
		return !matchesYear(e)
	})

	list = utils.Distinct(list, func(e *models.Entity) string {
		return strconv.FormatInt(e.ID, 10)
	})

	if len(list) == 0 {
		return saveAndReturn(nil)
	}
	if len(list) == 1 {
		return saveAndReturn(list[0])
	}

	// Если найдено несколько кандидатов, сортируем по соответствию года, совпадению названия и популярности
	sort.Slice(list, func(i, j int) bool {
		yI := tmdb.GetEntityYear(list[i])
		yJ := tmdb.GetEntityYear(list[j])
		exactYearI := (yI == yearml && yearml > 0)
		exactYearJ := (yJ == yearml && yearml > 0)
		if exactYearI != exactYearJ {
			return exactYearI
		}

		cleanM := utils.ClearStr(movie.Title)
		matchI := utils.ClearStr(list[i].Title) == cleanM || utils.ClearStr(list[i].OriginalTitle) == cleanM
		matchJ := utils.ClearStr(list[j].Title) == cleanM || utils.ClearStr(list[j].OriginalTitle) == cleanM
		if matchI != matchJ {
			return matchI
		}

		return list[i].VoteCount > list[j].VoteCount
	})

	return saveAndReturn(list[0])
}
