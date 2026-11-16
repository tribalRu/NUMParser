package rutor

import (
	"NUMParser/config"
	"NUMParser/db/models"
	"NUMParser/db/storage"
	"NUMParser/db/torrsearch"
	"compress/flate"
	"encoding/json"
	bolt "go.etcd.io/bbolt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	torrs         []*models.TorrentDetails
	hashIndex     map[string]int
	dirtyHashes   map[string]struct{}
	IsTorrsChange bool
	muTorrs       sync.Mutex
)

func Init() {
	storage.DB.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("Rutor"))
		if bucket == nil {
			return nil
		}
		bucket = bucket.Bucket([]byte("Torrents"))
		if bucket == nil {
			return nil
		}
		stats := bucket.Stats()
		if stats.KeyN > 0 {
			torrs = make([]*models.TorrentDetails, 0, stats.KeyN)
		}
		err := bucket.ForEach(func(_, v []byte) error {
			var torr *models.TorrentDetails
			err := json.Unmarshal(v, &torr)
			if err == nil {
				torrs = append(torrs, torr)
			}
			return err
		})
		if err != nil {
			log.Println("Error read rutor from db:", err)
		}
		return nil
	})

	dirtyHashes = make(map[string]struct{})
	rebuildHashIndex()
	torrsearch.NewIndex(torrs)

	// Check legacy rutor.ls in root or ensure releases/rutor.ls exists
	destPath := filepath.Join(config.SaveReleasePath, "rutor.ls")
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		legacyPath := "rutor.ls"
		if _, err := os.Stat(legacyPath); err == nil {
			_ = os.MkdirAll(config.SaveReleasePath, 0755)
			if err := os.Rename(legacyPath, destPath); err == nil {
				log.Printf("Migrated legacy %s to %s\n", legacyPath, destPath)
			}
		} else if len(torrs) > 0 {
			// Generate rutor.ls immediately from loaded DB torrs if it doesn't exist yet
			saveRutorLS(torrs)
		}
	}
}

func rebuildHashIndex() {
	hashIndex = make(map[string]int, len(torrs))
	for i, t := range torrs {
		if t.Hash != "" {
			hashIndex[t.Hash] = i
		}
	}
}

func RemoveAll() {
	muTorrs.Lock()
	torrs = nil
	hashIndex = nil
	dirtyHashes = make(map[string]struct{})
	IsTorrsChange = false
	muTorrs.Unlock()

	storage.DB.Update(func(tx *bolt.Tx) error {
		tx.DeleteBucket([]byte("Rutor"))
		return nil
	})
}

func GetTorrs() []*models.TorrentDetails {
	muTorrs.Lock()
	defer muTorrs.Unlock()
	res := make([]*models.TorrentDetails, len(torrs))
	copy(res, torrs)
	return res
}

func SetTorrs(list []*models.TorrentDetails) {
	muTorrs.Lock()
	defer muTorrs.Unlock()
	torrs = list
	rebuildHashIndex()
	dirtyHashes = make(map[string]struct{}, len(torrs))
	for _, t := range torrs {
		if t.Hash != "" {
			dirtyHashes[strings.ToLower(t.Hash)] = struct{}{}
		}
	}
	IsTorrsChange = true
}

func AddTorr(t *models.TorrentDetails) {
	if t == nil {
		return
	}
	if t.CreateDate.Year() < 1970 || t.CreateDate.Year() > 2100 {
		t.CreateDate = time.Now()
	}

	muTorrs.Lock()
	defer muTorrs.Unlock()

	if hashIndex == nil {
		hashIndex = make(map[string]int, len(torrs)+1)
	}
	if dirtyHashes == nil {
		dirtyHashes = make(map[string]struct{})
	}

	lowerHash := strings.ToLower(t.Hash)
	if t.Hash != "" {
		if i, ok := hashIndex[t.Hash]; ok {
			t.IMDBID = torrs[i].IMDBID
			if torrs[i].Seed != t.Seed || torrs[i].Peer != t.Peer || torrs[i].Title != t.Title {
				torrs[i] = t
				dirtyHashes[lowerHash] = struct{}{}
				IsTorrsChange = true
			}
			return
		}
	}

	IsTorrsChange = true
	if t.Hash != "" {
		hashIndex[t.Hash] = len(torrs)
		dirtyHashes[lowerHash] = struct{}{}
	}
	torrs = append(torrs, t)
}

func SaveTorrs() {
	muTorrs.Lock()
	supersededHashes := removeOldTorrLocked()
	if (!IsTorrsChange && len(supersededHashes) == 0) || len(torrs) == 0 {
		muTorrs.Unlock()
		return
	}

	var toSave []*models.TorrentDetails
	if len(dirtyHashes) > 0 {
		toSave = make([]*models.TorrentDetails, 0, len(dirtyHashes))
		for _, t := range torrs {
			if t.Hash == "" {
				continue
			}
			if _, ok := dirtyHashes[strings.ToLower(t.Hash)]; ok {
				toSave = append(toSave, t)
			}
		}
	} else if IsTorrsChange {
		toSave = make([]*models.TorrentDetails, len(torrs))
		copy(toSave, torrs)
	}

	torrsCopy := make([]*models.TorrentDetails, len(torrs))
	copy(torrsCopy, torrs)
	muTorrs.Unlock()

	// Prune superseded hashes from DB if any
	if len(supersededHashes) > 0 {
		_ = storage.DB.Update(func(tx *bolt.Tx) error {
			bucket := tx.Bucket([]byte("Rutor"))
			if bucket == nil {
				return nil
			}
			bucket = bucket.Bucket([]byte("Torrents"))
			if bucket == nil {
				return nil
			}
			for _, h := range supersededHashes {
				_ = bucket.Delete([]byte(h))
			}
			return nil
		})
	}

	if len(toSave) > 0 {
		log.Printf("Save torrents: writing %d dirty/new torrents (total in memory: %d)...", len(toSave), len(torrsCopy))
		t0 := time.Now()

		const batchSize = 2000
		savedCount := 0

		for i := 0; i < len(toSave); i += batchSize {
			if storage.IsStopping() {
				log.Println("Save torrents: shutdown signal received, pausing DB writes")
				break
			}

			end := i + batchSize
			if end > len(toSave) {
				end = len(toSave)
			}
			chunk := toSave[i:end]

			err := storage.DB.Update(func(tx *bolt.Tx) error {
				bucket, err := tx.CreateBucketIfNotExists([]byte("Rutor"))
				if err != nil {
					return err
				}
				bucket, err = bucket.CreateBucketIfNotExists([]byte("Torrents"))
				if err != nil {
					return err
				}

				for _, torr := range chunk {
					if torr.Hash == "" {
						continue
					}
					if torr.CreateDate.Year() < 1970 || torr.CreateDate.Year() > 2100 {
						torr.CreateDate = time.Now()
					}
					buf, err := json.Marshal(torr)
					if err != nil {
						log.Println("Error marshalling torrent:", torr.Hash, err)
						continue
					}
					err = bucket.Put([]byte(strings.ToLower(torr.Hash)), buf)
					if err != nil {
						return err
					}
				}
				return nil
			})

			if err != nil {
				log.Println("Error write batch to db rutor:", err)
				break
			}

			savedCount += len(chunk)
			if len(toSave) > batchSize && (savedCount%10000 == 0 || savedCount == len(toSave)) {
				log.Printf("Database progress: %d/%d torrents saved (%.1f%%)",
					savedCount, len(toSave), float64(savedCount)/float64(len(toSave))*100)
			}
		}

		log.Printf("Save torrents completed in %v (%d items written)", time.Since(t0), savedCount)

		muTorrs.Lock()
		for _, t := range toSave[:savedCount] {
			delete(dirtyHashes, strings.ToLower(t.Hash))
		}
		if len(dirtyHashes) == 0 {
			IsTorrsChange = false
		}
		muTorrs.Unlock()
	} else {
		muTorrs.Lock()
		IsTorrsChange = false
		muTorrs.Unlock()
	}

	saveRutorLS(torrsCopy)
	torrsearch.NewIndex(torrsCopy)
}

func removeOldTorrLocked() []string {
	list := make([]*models.TorrentDetails, 0, len(torrs))
	var droppedHashes []string
	inResult := make(map[string]struct{}, len(torrs))

	// Iterate in reverse so newest torrent for the same link is kept
	for i := len(torrs) - 1; i >= 0; i-- {
		t := torrs[i]
		if _, ok := inResult[t.Link]; !ok {
			inResult[t.Link] = struct{}{}
			list = append(list, t)
		} else {
			if t.Hash != "" {
				droppedHashes = append(droppedHashes, strings.ToLower(t.Hash))
			}
		}
	}

	// Reverse list back to preserve chronological order
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}

	torrs = list
	rebuildHashIndex()
	return droppedHashes
}

func saveRutorLS(torrsList []*models.TorrentDetails) {
	log.Println("Save torrents rutor.ls")
	destDir := config.SaveReleasePath
	_ = os.MkdirAll(destDir, 0755)

	destPath := filepath.Join(destDir, "rutor.ls")
	tempPath := destPath + ".tmp"

	ff, err := os.Create(tempPath)
	if err != nil {
		log.Println("Error creating temp rutor.ls:", err)
		return
	}

	w, err := flate.NewWriter(ff, flate.DefaultCompression)
	if err != nil {
		ff.Close()
		log.Println("Error creating flate writer:", err)
		return
	}

	rutorHost := config.RutorHost()
	out := make([]*models.TorrentDetails, len(torrsList))
	for i, t := range torrsList {
		cp := *t
		if cp.CreateDate.Year() < 1970 || cp.CreateDate.Year() > 2100 {
			cp.CreateDate = time.Now()
		}
		cp.Link = config.JoinRutorLink(rutorHost, cp.Link)
		out[i] = &cp
	}

	enc := json.NewEncoder(w)
	if err := enc.Encode(out); err != nil {
		w.Close()
		ff.Close()
		log.Println("Error encoding torrs:", err)
		return
	}

	if err := w.Close(); err != nil {
		ff.Close()
		log.Println("Error closing flate writer:", err)
		return
	}
	if err := ff.Close(); err != nil {
		log.Println("Error closing file:", err)
		return
	}

	if err := os.Rename(tempPath, destPath); err != nil {
		_ = os.Remove(destPath)
		if err := os.Rename(tempPath, destPath); err != nil {
			log.Println("Error renaming temp to rutor.ls:", err)
		}
	}
}
