package client

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	proxyIndex   atomic.Uint32
	proxyCacheMu sync.RWMutex
	proxyCache   []string
	proxyLoaded  time.Time
)

func loadProxyList() []string {
	proxyCacheMu.RLock()
	if time.Since(proxyLoaded) < 30*time.Second && proxyCache != nil {
		list := proxyCache
		proxyCacheMu.RUnlock()
		return list
	}
	proxyCacheMu.RUnlock()

	dir := filepath.Dir(os.Args[0])
	fileName := filepath.Join(dir, "proxy.list")
	buf, err := os.ReadFile(fileName)
	if err != nil {
		return nil
	}

	var list []string
	for _, proxy := range strings.Split(string(buf), "\n") {
		proxy = strings.TrimSpace(proxy)
		if proxy != "" {
			list = append(list, proxy)
		}
	}

	proxyCacheMu.Lock()
	proxyCache = list
	proxyLoaded = time.Now()
	proxyCacheMu.Unlock()

	return list
}

func getProxyFromList() string {
	list := loadProxyList()
	if len(list) == 0 {
		return ""
	}

	idx := int(proxyIndex.Add(1)-1) % len(list)
	proxyHost := list[idx]
	if !strings.HasPrefix(proxyHost, "http") && !strings.HasPrefix(proxyHost, "socks") {
		proxyHost = "//" + proxyHost
	}
	return proxyHost
}
