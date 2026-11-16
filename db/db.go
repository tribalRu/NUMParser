package db

import (
	"NUMParser/db/models"
	"NUMParser/db/rutor"
	"NUMParser/db/storage"
	"log"
)

func Init() {
	log.Println("Open db...")
	storage.Init()

	log.Println("Read db...")
	rutor.Init()
}

func RequestStop() {
	storage.RequestStop()
}

func Close() {
	log.Println("Closing db...")
	storage.Close()
}

func SaveAll() {
	rutor.SaveTorrs()
}

func GetTorrs() []*models.TorrentDetails {
	return rutor.GetTorrs()
}
