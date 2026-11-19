package releases

import (
	"NUMParser/db/models"
	"NUMParser/db/storage"
	tmdb2 "NUMParser/db/tmdb"
	"NUMParser/movies/tmdb"
	"NUMParser/parser"
	"NUMParser/utils"
	"bytes"
	"log"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/PuerkitoBio/goquery"
)

func FillTMDB(label string, isMovie bool, torrs []*models.TorrentDetails, limit int) []*models.Entity {
	list := make([]*models.Entity, len(torrs))
	var found atomic.Int32
	utils.PForLim(torrs, 6, func(i int, t *models.TorrentDetails) bool {
		if storage.IsStopping() {
			return false
		}
		var md *models.Entity
		fromCache := false

		if tmdb2.IsNotFound(t.Hash) {
			// Checked on TMDB within TTL and not found; skip network queries
			return true
		}

		indx := tmdb2.GetIndex(t.Hash)
		if indx != 0 {
			md = tmdb.GetVideoDetails(isMovie, indx)
			if md != nil {
				fromCache = true
			}
		}
		if md == nil {
			md = FindTMDBID(isMovie, t)
			if md == nil {
				md = FindTMDB(isMovie, t)
			}
			if md == nil {
				tmdb2.SetNotFound(t.Hash)
			}
		}
		if md == nil {
			log.Println(label+":", "Torr", i, "/", len(torrs), "not found in TMDB:", t.Title, t.Link)
		} else {
			list[i] = md
			f := found.Add(1)
			if !fromCache {
				tmdb2.SetIndex(t, md)
				log.Println(label+":", "Find torr", i, "/", len(torrs), "in TMDB (API):", t.Title)
			} else {
				log.Println(label+":", "Find torr", i, "/", len(torrs), "in cache:", t.Title)
			}
			md.SetTorrent(t)
			if limit > 0 && int(f) >= limit {
				return false
			}
		}
		return true
	})

	var result []*models.Entity
	for _, md := range list {
		if md != nil {
			result = append(result, md)
		}
	}
	EnrichEntitiesBatch(result)
	return result
}

func FindTMDBID(isMovie bool, torr *models.TorrentDetails) *models.Entity {
	if torr.IMDBID != "" {
		return tmdb.FindByID(isMovie, torr.IMDBID, "imdb_id")
	}
	body := parser.GetBodyLink(torr)
	if body == "" {
		return nil
	}

	imdbID, kpID := extractIDsFromHTML(body)
	if imdbID == "" && kpID == "" {
		return nil
	}

	torr.IMDBID = imdbID
	torr.KPID = kpID

	if imdbID != "" {
		return tmdb.FindByID(isMovie, imdbID, "imdb_id")
	}
	return nil
}

func extractIDsFromHTML(body string) (imdbID, kpID string) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewBufferString(body))
	if err != nil {
		return "", ""
	}

	doc.Find("table#details").Find("a").Each(func(i int, selection *goquery.Selection) {
		if link, ok := selection.Attr("href"); ok {
			if strings.Contains(link, "imdb.com") {
				link = strings.TrimRight(link, "/")
				if idx := strings.IndexAny(link, "?#"); idx != -1 {
					link = link[:idx]
				}
				link = strings.TrimRight(link, "/")
				arr := strings.Split(link, "/")
				if len(arr) > 0 {
					imdbID = arr[len(arr)-1]
				}
			}
			if strings.Contains(link, "kinopoisk.ru") {
				link = strings.TrimRight(link, "/")
				if idx := strings.IndexAny(link, "?#"); idx != -1 {
					link = link[:idx]
				}
				link = strings.TrimRight(link, "/")
				arr := strings.Split(link, "/")
				if len(arr) > 0 {
					kpID = arr[len(arr)-1]
				}
			}
		}
	})
	return imdbID, kpID
}

func FindTMDB(isMovie bool, torr *models.TorrentDetails) *models.Entity {
	list := tmdb.Search(isMovie, torr.Name)

	matchesYear := func(e *models.Entity) bool {
		y := tmdb.GetEntityYear(e)
		if y == 0 || torr.Year == 0 {
			return true
		}
		return utils.Abs(y-torr.Year) <= 1
	}

	list = utils.Filter(list, func(i int, e *models.Entity) bool {
		return !matchesYear(e)
	})

	if len(list) == 1 {
		return list[0]
	}

	for _, name := range torr.Names {
		lst := tmdb.Search(isMovie, name)
		list = append(list, lst...)
	}

	list = utils.Filter(list, func(i int, e *models.Entity) bool {
		return !matchesYear(e)
	})

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
		names := append([]string{torr.Name}, torr.Names...)
		list = utils.Filter(list, func(i int, e *models.Entity) bool {
			finds := 0
			for _, name := range names {
				if utils.ClearStr(e.Title) == utils.ClearStr(e.OriginalTitle) &&
					utils.ClearStr(name) == utils.ClearStr(e.Title) && len(names) == 1 {
					return false
				} else if utils.ClearStr(e.OriginalTitle) == utils.ClearStr(name) ||
					utils.ClearStr(e.Title) == utils.ClearStr(name) {
					finds++
				}
				if finds > 1 {
					return false
				}
				if utils.ClearStr(e.Title) != utils.ClearStr(e.OriginalTitle) && len(names) > 1 {
					for _, title := range e.Titles {
						if utils.ClearStr(title) == utils.ClearStr(name) {
							return false
						}
					}
				}
			}

			return true
		})
		if len(list) == 1 {
			return list[0]
		}
	}
	return nil
}
