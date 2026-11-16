package storage

import (
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	DB         *bolt.DB
	isStopping atomic.Bool
)

func IsStopping() bool {
	return isStopping.Load()
}

func RequestStop() {
	isStopping.Store(true)
}

func Init() {
	dir := filepath.Dir(os.Args[0])
	dir = filepath.Join(dir, "db")
	_ = os.MkdirAll(dir, 0755)
	fileDb := filepath.Join(dir, "numparser.db")
	var err error
	DB, err = bolt.Open(fileDb, 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		log.Fatalln("Error opening db:", err)
	}
}

func Close() {
	RequestStop()
	if DB != nil {
		done := make(chan error, 1)
		go func() {
			done <- DB.Close()
		}()
		select {
		case err := <-done:
			if err != nil {
				log.Println("Error closing db:", err)
			}
		case <-time.After(3 * time.Second):
			log.Println("Warning: db.Close() timed out after 3s")
		}
	}
}
