package client

import (
	"sync"
	"testing"
	"time"
)

func TestGetProxyConcurrent(t *testing.T) {
	proxyCacheMu.Lock()
	proxyCache = []string{"http://proxy1:8080", "http://proxy2:8080", "socks5://proxy3:1080"}
	proxyLoaded = time.Now()
	proxyCacheMu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := getProxyFromList()
			if p == "" {
				t.Errorf("expected non-empty proxy")
			}
		}()
	}
	wg.Wait()
}
