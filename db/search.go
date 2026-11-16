package db

import (
	"NUMParser/db/models"
	"NUMParser/db/torrsearch"
	"NUMParser/utils"
	"github.com/agnivade/levenshtein"
	"sort"
	"strconv"
	"strings"
)

//func indexTorrs() {
//	if IsTorrsChange {
//		torrsearch.NewIndex(GetTorrs())
//	}
//}

func SearchTorr(query string) []*models.TorrentDetails {
	list := torrsearch.Search(query)
	if len(list) == 0 {
		return nil
	}

	hash := utils.ClearStr(query)

	type sortKey struct {
		torr *models.TorrentDetails
		lev  int
	}
	keys := make([]sortKey, len(list))
	for i, t := range list {
		lhash := utils.ClearStr(strings.ToLower(t.Name+t.GetNames())) + strconv.Itoa(t.Year)
		keys[i] = sortKey{torr: t, lev: levenshtein.ComputeDistance(hash, lhash)}
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].lev == keys[j].lev {
			return keys[j].torr.CreateDate.Before(keys[i].torr.CreateDate)
		}
		return keys[i].lev < keys[j].lev
	})

	for i := range keys {
		list[i] = keys[i].torr
	}
	return list
}
