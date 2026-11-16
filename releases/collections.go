package releases

import (
	"NUMParser/config"
	"NUMParser/db"
	"NUMParser/db/models"
	"NUMParser/ml"
	"NUMParser/movies/tmdb"
	"NUMParser/releases/cover"
	"NUMParser/utils"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func GetCollections() {
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

		var ents []*models.Entity
		var found []*models.Entity

		fmt.Println("Search tmdb ids:")
		for _, movie := range movies {
			fmt.Printf("%s (%s) == ", movie.Title, movie.Year)
			e := findTmdbMlMovie(movie)
			if e != nil {
				fmt.Printf("%s | %s (%v): %v, %.2f\n", e.Title, e.OriginalTitle, e.Year, e.ID, e.VoteAverage*math.Log(float64(e.VoteCount)))
				ents = append(ents, e)
			} else {
				fmt.Println("TMDB not found")
			}
		}

		fmt.Println("Found tmdb for coll:", coll.Title, len(ents))
		if len(ents) > 0 {
			fmt.Println("Search torrents for cols")
			for i, e := range ents {
				list := db.SearchTorr(e.Title + " " + e.OriginalTitle + " " + e.Year)
				if len(list) == 0 {
					names := getEnNames(e.Titles)
					for _, name := range names {
						list = db.SearchTorr(e.Title + " " + name + " " + e.Year)
						if len(list) > 0 {
							break
						}
					}
				}
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

					e.SetTorrent(list[0])
					found = append(found, e)
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
				}
			}
		}

		fmt.Println()
		// Polite delay between collections to avoid exceeding rate limits (15 RPM for Gemini, 30 RPM for Groq)
		time.Sleep(2 * time.Second)
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

func getCollectionId(coll *ml.Collection, ents []*models.Entity) *CollectionId {
	rid := &CollectionId{
		Name:         coll.Title,
		Overview:     coll.Overview,
		Parts:        nil,
		PosterPath:   "",
		BackdropPath: "",
	}

	rutorHost := config.RutorHost()
	for _, e := range ents {
		if e != nil && e.GetTorrent() != nil {
			var countries []string
			if len(e.ProductionCountries) > 0 {
				for _, c := range e.ProductionCountries {
					countries = append(countries, c.Iso31661)
				}
			} else {
				countries = e.OriginCountry
			}
			d := e.GetTorrent()
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
				Id:          e.ID,
				MediaType:   e.MediaType,
				GenreIds:    e.GenresIds,
				VoteAverage: e.VoteAverage,
				VoteCount:   e.VoteCount,
				Countries:   countries,
				ReleaseDate: e.ReleaseDate,
				Torrent:     []*Torrent{&t},
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
	list := tmdb.Search(true, movie.Title)
	yearml, _ := strconv.Atoi(movie.Year)

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

	if len(list) == 1 {
		return list[0]
	}

	list = utils.Distinct(list, func(e *models.Entity) string {
		return strconv.FormatInt(e.ID, 10)
	})

	if len(list) == 1 {
		return list[0]
	}

	if len(list) > 0 {
		list = utils.Filter(list, func(i int, e *models.Entity) bool {
			return !matchesYear(e)
		})

		list = utils.Filter(list, func(i int, e *models.Entity) bool {
			if utils.ClearStr(e.Title) == utils.ClearStr(e.OriginalTitle) &&
				utils.ClearStr(movie.Title) == utils.ClearStr(e.Title) {
				return false
			} else if utils.ClearStr(e.OriginalTitle) == utils.ClearStr(movie.Title) ||
				utils.ClearStr(e.Title) == utils.ClearStr(movie.Title) {
				return false
			}
			return true
		})
		if len(list) == 1 {
			return list[0]
		}
	}
	return nil
}
