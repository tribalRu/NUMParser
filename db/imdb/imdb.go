package imdb

import (
	"NUMParser/config"
	"NUMParser/db/models"
	"NUMParser/utils"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type TitleDetails struct {
	Tconst         string   `json:"tconst"`
	TitleType      string   `json:"title_type"`
	PrimaryTitle   string   `json:"primary_title"`
	OriginalTitle  *string  `json:"original_title"`
	IsAdult        bool     `json:"is_adult"`
	StartYear      *int     `json:"start_year"`
	EndYear        *int     `json:"end_year"`
	RuntimeMinutes *int     `json:"runtime_minutes"`
	Genres         []string `json:"genres"`
	AverageRating  *float64 `json:"average_rating"`
	NumVotes       *int     `json:"num_votes"`
}

var (
	titleCache  sync.Map // tconst -> *TitleDetails
	searchCache sync.Map // query:year -> *TitleDetails

	clientOnce sync.Once
	httpClient *http.Client

	cbMu            sync.RWMutex
	consecutiveErrs int
	cbOpenUntil     time.Time
)

// IsCircuitBreakerOpen returns true if imdb-db is currently in temporary cooldown due to consecutive connection failures.
func IsCircuitBreakerOpen() bool {
	cbMu.RLock()
	defer cbMu.RUnlock()
	return time.Now().Before(cbOpenUntil)
}

func recordSuccess() {
	cbMu.Lock()
	defer cbMu.Unlock()
	consecutiveErrs = 0
	cbOpenUntil = time.Time{}
}

func recordFailure(err error) {
	cbMu.Lock()
	defer cbMu.Unlock()
	consecutiveErrs++
	if consecutiveErrs >= 2 {
		cbOpenUntil = time.Now().Add(60 * time.Second)
		log.Printf("[IMDb] Сервер базы недоступен (%v). Отключаем запросы к IMDb на 60 сек для быстрой работы парсера.\n", err)
	}
}

func getHTTPClient(timeoutSec int) *http.Client {
	if timeoutSec <= 0 {
		timeoutSec = 1
	}
	clientOnce.Do(func() {
		tr := &http.Transport{
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   50,
			IdleConnTimeout:       60 * time.Second,
			TLSHandshakeTimeout:   2 * time.Second,
			ResponseHeaderTimeout: time.Duration(timeoutSec) * time.Second,
		}
		httpClient = &http.Client{
			Transport: tr,
			Timeout:   time.Duration(timeoutSec) * time.Second,
		}
	})
	return httpClient
}

func newRequest(ctx context.Context, method, fullURL string, cfg config.ImdbConfig) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "NUMParser/2.0")
	req.Header.Set("Accept", "application/json")
	if cfg.Token != "" {
		token := cfg.Token
		if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
			token = "Bearer " + token
		}
		req.Header.Set("Authorization", token)
	}
	return req, nil
}

// Ping verifies whether the local imdb-db server is accessible.
func Ping(ctx context.Context) error {
	cfg := config.GetImdbConfig()
	if cfg.Host == "" {
		return fmt.Errorf("imdb host is empty")
	}

	endpoint := strings.TrimRight(cfg.Host, "/") + "/health"
	req, err := newRequest(ctx, "GET", endpoint, cfg)
	if err != nil {
		return err
	}

	cli := getHTTPClient(cfg.TimeoutSec)
	resp, err := cli.Do(req)
	if err != nil {
		recordFailure(err)
		return fmt.Errorf("imdb-db connection failed: %w", err)
	}
	defer resp.Body.Close()
	recordSuccess()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("imdb-db returned HTTP %d", resp.StatusCode)
	}

	return nil
}

// GetTitle fetches details for a single IMDb ID (e.g. "tt0111161").
func GetTitle(ctx context.Context, tconst string) (*TitleDetails, error) {
	tconst = strings.TrimSpace(tconst)
	if tconst == "" {
		return nil, fmt.Errorf("empty tconst")
	}

	if val, ok := titleCache.Load(tconst); ok {
		details, _ := val.(*TitleDetails)
		return details, nil
	}

	if IsCircuitBreakerOpen() {
		return nil, fmt.Errorf("imdb-db circuit breaker open")
	}

	cfg := config.GetImdbConfig()
	if !cfg.Enabled || cfg.Host == "" {
		return nil, fmt.Errorf("imdb integration disabled")
	}

	endpoint := fmt.Sprintf("%s/api/titles/%s", strings.TrimRight(cfg.Host, "/"), url.PathEscape(tconst))
	req, err := newRequest(ctx, "GET", endpoint, cfg)
	if err != nil {
		return nil, err
	}

	cli := getHTTPClient(cfg.TimeoutSec)
	resp, err := cli.Do(req)
	if err != nil {
		recordFailure(err)
		return nil, err
	}
	defer resp.Body.Close()
	recordSuccess()

	if resp.StatusCode == http.StatusNotFound {
		titleCache.Store(tconst, (*TitleDetails)(nil))
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("imdb-db returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var details TitleDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, fmt.Errorf("failed to decode imdb response: %w", err)
	}

	titleCache.Store(tconst, &details)
	return &details, nil
}

// GetTitlesBatch retrieves details for multiple IMDb IDs using batching.
func GetTitlesBatch(ctx context.Context, tconsts []string) (map[string]*TitleDetails, error) {
	result := make(map[string]*TitleDetails)
	var missing []string

	for _, tc := range tconsts {
		tc = strings.TrimSpace(tc)
		if tc == "" {
			continue
		}
		if val, ok := titleCache.Load(tc); ok {
			if details, _ := val.(*TitleDetails); details != nil {
				result[tc] = details
			}
		} else {
			missing = append(missing, tc)
		}
	}

	if len(missing) == 0 {
		return result, nil
	}

	if IsCircuitBreakerOpen() {
		return result, fmt.Errorf("imdb-db circuit breaker open")
	}

	cfg := config.GetImdbConfig()
	if !cfg.Enabled || cfg.Host == "" {
		return result, nil
	}

	// Chunk queries into groups of 300 (fast batching)
	const chunkSize = 300
	cli := getHTTPClient(cfg.TimeoutSec)
	baseURL := strings.TrimRight(cfg.Host, "/") + "/api/titles"

	for i := 0; i < len(missing); i += chunkSize {
		if IsCircuitBreakerOpen() {
			break
		}
		end := i + chunkSize
		if end > len(missing) {
			end = len(missing)
		}
		chunk := missing[i:end]

		params := url.Values{}
		for _, tc := range chunk {
			params.Add("tconst", tc)
		}

		reqURL := baseURL + "?" + params.Encode()
		req, err := newRequest(ctx, "GET", reqURL, cfg)
		if err != nil {
			log.Printf("[IMDb] Error creating batch request: %v\n", err)
			continue
		}

		resp, err := cli.Do(req)
		if err != nil {
			recordFailure(err)
			log.Printf("[IMDb] Batch request error: %v\n", err)
			continue
		}
		recordSuccess()

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		var items []TitleDetails
		err = json.NewDecoder(resp.Body).Decode(&items)
		resp.Body.Close()
		if err != nil {
			log.Printf("[IMDb] Failed to decode batch response: %v\n", err)
			continue
		}

		foundSet := make(map[string]bool)
		for _, item := range items {
			itemCopy := item
			titleCache.Store(item.Tconst, &itemCopy)
			result[item.Tconst] = &itemCopy
			foundSet[item.Tconst] = true
		}

		// Cache not found for missing
		for _, tc := range chunk {
			if !foundSet[tc] {
				titleCache.Store(tc, (*TitleDetails)(nil))
			}
		}
	}

	return result, nil
}

// SearchTitle searches IMDb by title and year.
func SearchTitle(ctx context.Context, query string, year int) (*TitleDetails, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty query")
	}

	cacheKey := fmt.Sprintf("%s:%d", strings.ToLower(query), year)
	if val, ok := searchCache.Load(cacheKey); ok {
		details, _ := val.(*TitleDetails)
		return details, nil
	}

	if IsCircuitBreakerOpen() {
		return nil, fmt.Errorf("imdb-db circuit breaker open")
	}

	cfg := config.GetImdbConfig()
	if !cfg.Enabled || cfg.Host == "" {
		return nil, fmt.Errorf("imdb integration disabled")
	}

	params := url.Values{}
	params.Set("q", query)
	params.Set("title_type", "movie")
	if year > 0 {
		params.Set("year_from", strconv.Itoa(year-1))
	}

	endpoint := strings.TrimRight(cfg.Host, "/") + "/api/search/titles?" + params.Encode()
	req, err := newRequest(ctx, "GET", endpoint, cfg)
	if err != nil {
		return nil, err
	}

	cli := getHTTPClient(cfg.TimeoutSec)
	resp, err := cli.Do(req)
	if err != nil {
		recordFailure(err)
		return nil, err
	}
	defer resp.Body.Close()
	recordSuccess()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("imdb search returned HTTP %d", resp.StatusCode)
	}

	var searchResults []struct {
		Tconst        string   `json:"tconst"`
		PrimaryTitle  string   `json:"primary_title"`
		OriginalTitle *string  `json:"original_title"`
		StartYear     *int     `json:"start_year"`
		Genres        []string `json:"genres"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&searchResults); err != nil {
		return nil, err
	}

	if len(searchResults) == 0 {
		searchCache.Store(cacheKey, (*TitleDetails)(nil))
		return nil, nil
	}

	// Match best candidate by year and title similarity
	var bestTconst string
	cleanQ := utils.ClearStr(query)

	for _, item := range searchResults {
		if year > 0 && item.StartYear != nil && utils.Abs(*item.StartYear-year) <= 1 {
			cleanP := utils.ClearStr(item.PrimaryTitle)
			if cleanP == cleanQ {
				bestTconst = item.Tconst
				break
			}
			if item.OriginalTitle != nil && utils.ClearStr(*item.OriginalTitle) == cleanQ {
				bestTconst = item.Tconst
				break
			}
		}
	}

	if bestTconst == "" {
		bestTconst = searchResults[0].Tconst
	}

	// Fetch full details with rating
	details, err := GetTitle(ctx, bestTconst)
	if err != nil || details == nil {
		searchCache.Store(cacheKey, (*TitleDetails)(nil))
		return nil, err
	}

	searchCache.Store(cacheKey, details)
	return details, nil
}

// EnrichEntitiesBatch performs ultra-fast batch enrichment of hundreds or thousands of entities at once.
func EnrichEntitiesBatch(ctx context.Context, ents []*models.Entity) {
	if len(ents) == 0 {
		return
	}
	cfg := config.GetImdbConfig()
	if !cfg.Enabled || cfg.Host == "" || IsCircuitBreakerOpen() {
		return
	}

	var missing []string
	seen := make(map[string]bool)

	for _, e := range ents {
		if e == nil || e.ImdbRating > 0 {
			continue
		}
		if e.ImdbID == "" && e.GetTorrent() != nil && e.GetTorrent().IMDBID != "" {
			e.ImdbID = e.GetTorrent().IMDBID
		}
		if e.ImdbID != "" && !seen[e.ImdbID] {
			seen[e.ImdbID] = true
			if _, ok := titleCache.Load(e.ImdbID); !ok {
				missing = append(missing, e.ImdbID)
			}
		}
	}

	if len(missing) > 0 {
		_, _ = GetTitlesBatch(ctx, missing)
	}

	// Apply data from cache to entities in memory (0 ms)
	for _, e := range ents {
		if e == nil || e.ImdbID == "" || e.ImdbRating > 0 {
			continue
		}
		if val, ok := titleCache.Load(e.ImdbID); ok {
			details, _ := val.(*TitleDetails)
			if details == nil {
				continue
			}
			if details.AverageRating != nil && *details.AverageRating > 0 {
				e.ImdbRating = *details.AverageRating
				if details.NumVotes != nil {
					e.ImdbVotes = *details.NumVotes
				}
				e.RatingSource = "imdb"
				if cfg.PreferImdbRating {
					e.VoteAverage = e.ImdbRating
					e.VoteCount = e.ImdbVotes
				}
			}
			if details.RuntimeMinutes != nil && *details.RuntimeMinutes > 0 && e.Runtime == 0 {
				e.Runtime = *details.RuntimeMinutes
			}
		}
	}
}

// EnrichEntity enriches an Entity with IMDb ratings, votes, and metadata.
func EnrichEntity(ctx context.Context, ent *models.Entity) bool {
	if ent == nil || ent.ImdbRating > 0 {
		return ent != nil && ent.ImdbRating > 0
	}

	cfg := config.GetImdbConfig()
	if !cfg.Enabled || IsCircuitBreakerOpen() {
		return false
	}

	var details *TitleDetails
	var err error

	// 1. Поиск по ImdbID
	if ent.ImdbID == "" && ent.GetTorrent() != nil && ent.GetTorrent().IMDBID != "" {
		ent.ImdbID = ent.GetTorrent().IMDBID
	}
	if ent.ImdbID != "" {
		details, err = GetTitle(ctx, ent.ImdbID)
	}

	// 2. Если ImdbID не найден, пробуем поиск по названию только если цепь не разорвана
	if (details == nil || err != nil) && !IsCircuitBreakerOpen() && ent.OriginalTitle != "" {
		y, _ := strconv.Atoi(ent.Year)
		details, _ = SearchTitle(ctx, ent.OriginalTitle, y)
		if details == nil && ent.Title != "" {
			details, _ = SearchTitle(ctx, ent.Title, y)
		}
	}

	if details == nil {
		return false
	}

	if ent.ImdbID == "" && details.Tconst != "" {
		ent.ImdbID = details.Tconst
	}

	if details.AverageRating != nil && *details.AverageRating > 0 {
		ent.ImdbRating = *details.AverageRating
		if details.NumVotes != nil {
			ent.ImdbVotes = *details.NumVotes
		}
		ent.RatingSource = "imdb"

		if cfg.PreferImdbRating {
			ent.VoteAverage = ent.ImdbRating
			ent.VoteCount = ent.ImdbVotes
		}
	}

	if details.RuntimeMinutes != nil && *details.RuntimeMinutes > 0 && ent.Runtime == 0 {
		ent.Runtime = *details.RuntimeMinutes
	}

	return true
}
