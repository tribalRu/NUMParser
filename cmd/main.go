package main

import (
	"NUMParser/config"
	"NUMParser/db"
	"NUMParser/db/models"
	"NUMParser/db/storage"
	"NUMParser/ml"
	"NUMParser/movies/tmdb"
	"NUMParser/parser"
	"NUMParser/releases"
	"NUMParser/version"
	"NUMParser/web"
	"context"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"NUMParser/utils"

	"github.com/alexflint/go-arg"
	"github.com/jasonlvhit/gocron"
)

type args struct {
	Port       string `arg:"-p" help:"web server port, default 38888"`
	Proxy      string `arg:"--proxy" help:"proxy for rutor, http://user:password@ip:port"`
	UseProxy   bool   `arg:"--useproxy" help:"enable auto proxy"`
	GenColls   bool   `arg:"--gencolls" help:"force regenerate AI collections on startup"`
	ForceYears bool   `arg:"--forceyears" help:"force regenerate all movie years (1980..now) on startup"`
}

var (
	params               args
	isScanningReleases   atomic.Bool
	isScanningYears      atomic.Bool
	isScanningColls      atomic.Bool
	isGeneratingReleases atomic.Bool
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Configure runtime memory bounds for low-RAM (2GB VPS) environments:
	// Soft memory limit: 600MB. Go GC actively collects heap as memory approaches this boundary.
	debug.SetMemoryLimit(600 * 1024 * 1024)
	debug.SetGCPercent(50)

	arg.MustParse(&params)

	fmt.Println("=========== START ===========")
	fmt.Println("numParser_"+version.Version+",", runtime.Version()+",", "CPU Num:", runtime.NumCPU())

	if params.Port == "" {
		port, err := config.ReadConfigParser("Port")
		if err == nil {
			params.Port = port
		} else {
			params.Port = "38888"
		}
	}

	if params.Proxy != "" {
		proxyURL, err := url.Parse(params.Proxy)
		if err != nil {
			log.Println("Error parse proxy host:", err)
		} else if proxyURL.Scheme == "" || proxyURL.Host == "" {
			log.Println("Error parse proxy host: empty scheme or host")
		} else {
			config.ProxyHost = params.Proxy
		}
	} else {
		proxy, err := config.ReadConfigParser("Proxy")
		if err == nil {
			params.Proxy = proxy
			proxyURL, err := url.Parse(params.Proxy)
			if params.Proxy == "" {
				config.ProxyHost = ""
			} else if err == nil && proxyURL.Scheme != "" && proxyURL.Host != "" {
				config.ProxyHost = params.Proxy
			} else if err != nil {
				log.Println("Error parse proxy host:", err)
			} else {
				log.Println("Error parse proxy host: empty scheme or host")
			}
		}
	}

	if params.UseProxy == true {
		config.UseProxy = params.UseProxy
	} else {
		use_proxy, err := config.ReadConfigParser("UseProxy")
		if err == nil && use_proxy == "true" {
			params.UseProxy = true
		} else {
			params.UseProxy = false
		}
		config.UseProxy = params.UseProxy
	}

	dnsResolve()

	db.Init()
	loadProxy()
	tmdb.Init()
	ml.Init()

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	getDbInfo()
	web.Start(params.Port)

	log.Println("Start timer")
	gocron.Every(3).Hours().From(calcTime()).Do(scanReleases)
	gocron.Every(1).Day().At("2:30").Do(scanMoviesYears)
	gocron.Every(1).Friday().At("0:00").Do(scanCollections)

	go func() {
		<-gocron.Start()
	}()

	// Start initial background scanning so web UI is immediately responsive
	go func() {
		// 1. Immediately rebuild all 6 main categories + legends from local DB in 5-10 seconds!
		// No waiting for Rutor crawler: valid release files are available on disk immediately.
		rebuildReleasesNow()
		utils.FreeOSMemGC()

		// 2. Crawl Rutor tracker for fresh torrents in background
		scanReleases()
		utils.FreeOSMemGC()
		scanMoviesYears()
		utils.FreeOSMemGC()
		scanCollections()
		utils.FreeOSMemGC()
	}()

	sig := <-stopChan
	log.Println("Received signal:", sig, "- shutting down gracefully...")
	db.RequestStop()
	gocron.Clear()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := web.Stop(ctx); err != nil {
		log.Println("Error stopping web server:", err)
	}

	db.Close()
	log.Println("numParser stopped cleanly")
}

func dnsResolve() {
	hosts := [6]string{"1.1.1.1", "1.0.0.1", "208.67.222.222", "208.67.220.220", "8.8.8.8", "8.8.4.4"}
	ret := 0
	for _, ip := range hosts {
		ret = toolResolve("www.google.com", ip)
		switch {
		case ret == 2:
			fmt.Println("DNS resolver OK")
		case ret == 1:
			fmt.Println("New DNS resolver OK")
		case ret == 0:
			fmt.Println("New DNS resolver failed")
		}
		if ret == 2 || ret == 1 {
			break
		}
	}
}

func toolResolve(host string, serverDNS string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	addrs, _ := net.DefaultResolver.LookupHost(ctx, host)
	addr_dns := fmt.Sprintf("%s:53", serverDNS)
	a := 0
	if len(addrs) == 0 {
		fn := func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, "udp", addr_dns)
		}
		net.DefaultResolver = &net.Resolver{
			PreferGo: true,
			Dial:     fn,
		}
		ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel2()
		addrs, err := net.DefaultResolver.LookupHost(ctx2, host)
		if err == nil && len(addrs) > 0 {
			a = 1
		} else {
			a = 0
		}
	} else {
		a = 2
	}
	return a
}

func rebuildReleasesNow() {
	if isGeneratingReleases.Swap(true) {
		log.Println("rebuildReleasesNow: already in progress, skipping duplicate run")
		return
	}
	defer isGeneratingReleases.Store(false)

	log.Println("==================================================")
	log.Println("=== Immediate rebuild of all release categories ===")
	log.Println("==================================================")
	releases.GenerateAllReleases()
	releases.GetLegends()
	db.SaveAll()
	copySH()
	log.Println("==================================================")
	log.Println("=== All categories successfully rebuilt on disk ===")
	log.Println("==================================================")
}

func scanReleases() {
	if isScanningReleases.Swap(true) {
		log.Println("scanReleases: already in progress, skipping duplicate run")
		return
	}
	defer isScanningReleases.Store(false)

	loadProxy()
	rutorParser := parser.NewRutor()
	rutorParser.Parse()
	getDbInfo()

	if storage.IsStopping() {
		return
	}

	releases.GenerateAllReleases()
	db.SaveAll()
	copySH()
}

func scanMoviesYears() {
	if isScanningYears.Swap(true) {
		log.Println("scanMoviesYears: already in progress, skipping duplicate run")
		return
	}
	defer isScanningYears.Store(false)

	loadProxy()
	releases.GetLegends()

	torrs := db.GetTorrs()
	currentYear := time.Now().Year()
	var yearsToScan []int

	for y := 1980; y <= currentYear; y++ {
		if storage.IsStopping() {
			return
		}
		if params.ForceYears || releases.ShouldScanYear(y, torrs) {
			yearsToScan = append(yearsToScan, y)
		} else {
			log.Printf("Movies %d: up to date, skipping", y)
		}
	}

	log.Printf("scanMoviesYears: %d years to scan (out of %d total)", len(yearsToScan), currentYear-1980+1)

	// Process years sequentially to maintain a minimal memory footprint (~100-150MB) on 2GB RAM VPS
	for _, y := range yearsToScan {
		if storage.IsStopping() {
			break
		}
		releases.GetNewMoviesYear(y, torrs)
		utils.FreeOSMemGC()
	}

	db.SaveAll()
	copySH()
	utils.FreeOSMemGC()
}

func scanCollections() {
	if isScanningColls.Swap(true) {
		log.Println("scanCollections: already in progress, skipping duplicate run")
		return
	}
	defer isScanningColls.Store(false)

	if params.GenColls || time.Since(ml.CollsConfig.LastUpdated).Hours() > 160.0 {
		params.GenColls = false
		loadProxy()
		releases.GetCollections()
		db.SaveAll()
		copySH()
	}
}

func findScript(nameWithoutExt string) string {
	ext := ".sh"
	if runtime.GOOS == "windows" {
		ext = ".bat"
	}
	scriptName := nameWithoutExt + ext

	if _, err := os.Stat(scriptName); err == nil {
		return scriptName
	}

	execDir := filepath.Dir(os.Args[0])
	execScript := filepath.Join(execDir, scriptName)
	if _, err := os.Stat(execScript); err == nil {
		return execScript
	}

	return ""
}

// Exec script for copy any files
func copySH() {
	scriptPath := findScript("copy")
	if scriptPath == "" {
		return
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd.exe", "/C", scriptPath)
	} else {
		cmd = exec.Command("/bin/sh", scriptPath)
	}
	logOut, err := cmd.CombinedOutput()
	if err != nil {
		log.Println("Error copy releases:", err)
	}
	output := strings.TrimSpace(string(logOut))
	if len(output) > 0 {
		log.Println(output)
	}
}

// Exec script for load proxy, script must create file proxy.list
func loadProxy() {
	if !config.UseProxy {
		return
	}
	scriptPath := findScript("proxy")
	if scriptPath == "" {
		return
	}
	log.Println("Load proxy list...")
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd.exe", "/C", scriptPath)
	} else {
		cmd = exec.Command("/bin/sh", scriptPath)
	}
	logOut, err := cmd.CombinedOutput()
	if err != nil {
		log.Println("Error proxy releases:", err)
	}
	output := strings.TrimSpace(string(logOut))
	if len(output) > 0 {
		log.Println(output)
	}
}

func calcTime() *time.Time {
	// 2, 5, 8, 11, 14, 17, 20, 23
	hour := time.Now().Hour()
	t := time.Date(time.Now().Year(),
		time.Now().Month(),
		time.Now().Day(),
		0, 0, 0, 0, time.Local)
	switch {
	case hour < 2:
		t = t.Add(2 * time.Hour)
	case hour < 5:
		t = t.Add(5 * time.Hour)
	case hour < 8:
		t = t.Add(8 * time.Hour)
	case hour < 11:
		t = t.Add(11 * time.Hour)
	case hour < 14:
		t = t.Add(14 * time.Hour)
	case hour < 17:
		t = t.Add(17 * time.Hour)
	case hour < 20:
		t = t.Add(20 * time.Hour)
	case hour < 23:
		t = t.Add(23 * time.Hour)
	default:
		t = t.Add(26 * time.Hour)
	}
	return &t
}

func getDbInfo() {
	listTorr := db.GetTorrs()
	wIMDB := 0
	nIMDB := 0

	cMovie := 0
	cSeries := 0
	cDocMovie := 0
	cDocSeries := 0
	cCartoonMovie := 0
	cCartoonSeries := 0
	cTVShow := 0
	cAnime := 0

	for _, d := range listTorr {
		if d.IMDBID != "" {
			wIMDB++
		} else {
			nIMDB++
		}
		switch d.Categories {
		case models.CatMovie:
			cMovie++
		case models.CatSeries:
			cSeries++
		case models.CatDocMovie:
			cDocMovie++
		case models.CatDocSeries:
			cDocSeries++
		case models.CatCartoonMovie:
			cCartoonMovie++
		case models.CatCartoonSeries:
			cCartoonSeries++
		case models.CatTVShow:
			cTVShow++
		case models.CatAnime:
			cAnime++
		}
	}

	fmt.Println("Movies:", cMovie)
	fmt.Println("Serials:", cSeries)
	fmt.Println("Doc Movies:", cDocMovie)
	fmt.Println("Doc Serials:", cDocSeries)
	fmt.Println("Cartoons:", cCartoonMovie)
	fmt.Println("Cartoons Serial:", cCartoonSeries)
	fmt.Println("TV Show:", cTVShow)
	fmt.Println("Animes:", cAnime)

	fmt.Println("Torrents with IMDB:", wIMDB)
	fmt.Println("Torrents without IMDB:", nIMDB)
	fmt.Println("Torrents:", len(listTorr))
}
