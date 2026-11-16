package torrsearch

import (
	"NUMParser/db/models"
	"fmt"
	"sync"
	"testing"
)

func TestTorrsearchConcurrency(t *testing.T) {
	var sample []*models.TorrentDetails
	for i := 0; i < 50; i++ {
		sample = append(sample, &models.TorrentDetails{
			Title: fmt.Sprintf("Film Matrix Reloaded %d 1080p BDRip", i),
			Name:  "Matrix",
			Year:  2000 + i,
		})
	}

	NewIndex(sample)

	var wg sync.WaitGroup
	// Concurrently search and re-index
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(iter int) {
			defer wg.Done()
			results := Search("matrix")
			if len(results) == 0 {
				t.Errorf("expected results for matrix, got 0")
			}
			if iter%5 == 0 {
				NewIndex(sample)
			}
		}(i)
	}
	wg.Wait()
}
