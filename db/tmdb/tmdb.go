package tmdb

import (
	"NUMParser/db/models"
	"NUMParser/db/storage"
	"NUMParser/db/utils"
	"encoding/json"
	"errors"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	entCacheMu sync.RWMutex
	movieCache = make(map[int64]*models.Entity)
	tvCache    = make(map[int64]*models.Entity)
)

const maxEntityCache = 300

func getCachedEntity(cache map[int64]*models.Entity, id int64) *models.Entity {
	entCacheMu.RLock()
	ent, ok := cache[id]
	entCacheMu.RUnlock()
	if ok && ent != nil {
		cp := *ent
		return &cp
	}
	return nil
}

func setCachedEntity(cache map[int64]*models.Entity, ent *models.Entity) {
	if ent == nil {
		return
	}
	entCacheMu.Lock()
	if len(cache) >= maxEntityCache {
		count := 0
		for k := range cache {
			delete(cache, k)
			count++
			if count >= maxEntityCache/2 {
				break
			}
		}
	}
	cp := *ent
	cp.SetTorrent(nil)
	cache[ent.ID] = &cp
	entCacheMu.Unlock()
}

func GetMovie(id int64) *models.Entity {
	if ent := getCachedEntity(movieCache, id); ent != nil {
		return ent
	}
	if storage.DB == nil {
		return nil
	}
	var ent *models.Entity
	storage.DB.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("TMDB"))
		if bucket == nil {
			return nil
		}
		bucket = bucket.Bucket([]byte("Ents"))
		if bucket == nil {
			return nil
		}
		bucket = bucket.Bucket([]byte("Movies"))
		if bucket == nil {
			return nil
		}
		buf := bucket.Get(utils.I2B(id))
		if len(buf) == 0 {
			return nil
		}
		return json.Unmarshal(buf, &ent)
	})
	if ent != nil {
		setCachedEntity(movieCache, ent)
	}
	return ent
}

func GetTV(id int64) *models.Entity {
	if ent := getCachedEntity(tvCache, id); ent != nil {
		return ent
	}
	if storage.DB == nil {
		return nil
	}
	var ent *models.Entity
	storage.DB.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("TMDB"))
		if bucket == nil {
			return nil
		}
		bucket = bucket.Bucket([]byte("Ents"))
		if bucket == nil {
			return nil
		}
		bucket = bucket.Bucket([]byte("TV"))
		if bucket == nil {
			return nil
		}
		buf := bucket.Get(utils.I2B(id))
		if len(buf) == 0 {
			return nil
		}
		return json.Unmarshal(buf, &ent)
	})
	if ent != nil {
		setCachedEntity(tvCache, ent)
	}
	return ent
}

var errStopIter = errors.New("stop")

func FindIMDB(id string) *models.Entity {
	if id == "" || storage.DB == nil {
		return nil
	}

	// 1. Быстрый поиск через индекс O(1)
	var tmdbID int64
	storage.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("TMDB"))
		if b == nil {
			return nil
		}
		idxBucket := b.Bucket([]byte("IMDB_Index"))
		if idxBucket == nil {
			return nil
		}
		val := idxBucket.Get([]byte(id))
		if len(val) > 0 {
			tmdbID = utils.B2I(val)
		}
		return nil
	})

	if tmdbID != 0 {
		if ent := GetMovie(tmdbID); ent != nil {
			return ent
		}
		if ent := GetTV(tmdbID); ent != nil {
			return ent
		}
	}

	// 2. Фоллбек: линейное сканирование существующих записей
	var ent *models.Entity
	storage.DB.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("TMDB"))
		if bucket == nil {
			return nil
		}
		bucket = bucket.Bucket([]byte("Ents"))
		if bucket == nil {
			return nil
		}
		scan := func(name string) {
			b := bucket.Bucket([]byte(name))
			if b == nil {
				return
			}
			b.ForEach(func(_, v []byte) error {
				var e *models.Entity
				if err := json.Unmarshal(v, &e); err != nil {
					return nil
				}
				if e.ImdbID == id {
					ent = e
					return errStopIter
				}
				return nil
			})
		}
		scan("TV")
		if ent != nil {
			return nil
		}
		scan("Movies")
		return nil
	})

	// Сохраняем в индекс для быстрых последующих поисков
	if ent != nil && !storage.IsStopping() {
		_ = storage.DB.Update(func(tx *bolt.Tx) error {
			b, err := tx.CreateBucketIfNotExists([]byte("TMDB"))
			if err != nil {
				return err
			}
			idxBucket, err := b.CreateBucketIfNotExists([]byte("IMDB_Index"))
			if err != nil {
				return err
			}
			return idxBucket.Put([]byte(id), utils.I2B(ent.ID))
		})
	}

	return ent
}

func AddTMDB(t *models.Entity) {
	if storage.IsStopping() || storage.DB == nil {
		return
	}
	t.UpdateDate = time.Now()
	if t.MediaType == "movie" {
		setCachedEntity(movieCache, t)
	} else {
		setCachedEntity(tvCache, t)
	}
	storage.DB.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("TMDB"))
		if err != nil {
			return err
		}
		bucket, err = bucket.CreateBucketIfNotExists([]byte("Ents"))
		if err != nil {
			return err
		}
		if t.MediaType == "movie" {
			bucket, err = bucket.CreateBucketIfNotExists([]byte("Movies"))
		} else {
			bucket, err = bucket.CreateBucketIfNotExists([]byte("TV"))
		}
		if err != nil {
			return err
		}
		buf, err := json.Marshal(t)
		if err != nil {
			return err
		}
		if err := bucket.Put(utils.I2B(t.ID), buf); err != nil {
			return err
		}

		// Записываем во вторичный индекс IMDB -> TMDB ID
		if t.ImdbID != "" {
			tmdbBucket := tx.Bucket([]byte("TMDB"))
			if tmdbBucket != nil {
				idxBucket, err := tmdbBucket.CreateBucketIfNotExists([]byte("IMDB_Index"))
				if err == nil {
					_ = idxBucket.Put([]byte(t.ImdbID), utils.I2B(t.ID))
				}
			}
		}
		return nil
	})
}
