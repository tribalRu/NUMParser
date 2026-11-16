package torrsearch

import (
	"NUMParser/db/models"
	"sync"
)

type indexData struct {
	docs []*models.TorrentDetails
	m    map[string][]int
}

var (
	idxLock sync.RWMutex
	current *indexData
)

func NewIndex(torrs []*models.TorrentDetails) {
	docsCopy := make([]*models.TorrentDetails, len(torrs))
	copy(docsCopy, torrs)

	m := make(map[string][]int)
	for id, torr := range docsCopy {
		if torr == nil {
			continue
		}
		for _, token := range analyze(torr.Title) {
			ids := m[token]
			if len(ids) > 0 && ids[len(ids)-1] == id {
				continue
			}
			m[token] = append(ids, id)
		}
	}

	idxLock.Lock()
	current = &indexData{
		docs: docsCopy,
		m:    m,
	}
	idxLock.Unlock()
}

func Search(text string) []*models.TorrentDetails {
	idxLock.RLock()
	snap := current
	idxLock.RUnlock()

	if snap == nil || len(snap.docs) == 0 {
		return nil
	}

	var matchedIDs []int
	for _, token := range analyze(text) {
		ids, ok := snap.m[token]
		if !ok {
			return nil
		}
		if matchedIDs == nil {
			matchedIDs = ids
		} else {
			matchedIDs = intersection(matchedIDs, ids)
			if len(matchedIDs) == 0 {
				return nil
			}
		}
	}

	if len(matchedIDs) == 0 {
		return nil
	}

	res := make([]*models.TorrentDetails, 0, len(matchedIDs))
	for _, id := range matchedIDs {
		if id >= 0 && id < len(snap.docs) {
			res = append(res, snap.docs[id])
		}
	}
	return res
}

func intersection(a []int, b []int) []int {
	maxLen := len(a)
	if len(b) < maxLen {
		maxLen = len(b)
	}
	r := make([]int, 0, maxLen)
	var i, j int
	for i < len(a) && j < len(b) {
		if a[i] < b[j] {
			i++
		} else if a[i] > b[j] {
			j++
		} else {
			r = append(r, a[i])
			i++
			j++
		}
	}
	return r
}
