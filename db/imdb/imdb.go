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
)

func getHTTPClient(timeoutSec int) *http.Client {
	clientOnce.Do(func() {
		tr := &http.Transport{
			MaxIdleConns:        50,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     60 * time.Second,
			TLSHandshakeTimeout: 5 * time.Second,
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
		return fmt.Errorf("imdb-db connection failed: %w", err)
	}
	defer resp.Body.Close()

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
		if val == nil {
			return nil, nil
		}
		return val.(*TitleDetails), nil
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
		return nil, err
	}
	defer resp.Body.Close()

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
			if val != nil {
				result[tc] = val.(*TitleDetails)
			}
		} else {
			missing = append(missing, tc)
		}
	}

	if len(missing) == 0 {
		return result, nil
	}

	cfg := config.GetImdbConfig()
	if !cfg.Enabled || cfg.Host == "" {
		return result, nil
	}

	// Chunk queries into groups of 200 (imdb-db limit is 500)
	const chunkSize = 200
	cli := getHTTPClient(cfg.TimeoutSec)
	baseURL := strings.TrimRight(cfg.Host, "/") + "/api/titles"

	for i := 0; i < len(missing); i += chunkSize {
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
			log.Printf("[IMDb] Batch request error: %v\n", err)
			continue
		}

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
		if val == nil {
			return nil, nil
		}
		return val.(*TitleDetails), nil
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
		return nil, err
	}
	defer resp.Body.Close()

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

// EnrichEntity enriches an Entity with IMDb ratings, votes, and metadata.
func EnrichEntity(ctx context.Context, ent *models.Entity) bool {
	if ent == nil {
		return false
	}

	cfg := config.GetImdbConfig()
	if !cfg.Enabled {
		return false
	}

	var details *TitleDetails
	var err error

	// 1. Поиск по ImdbID
	if ent.ImdbID != "" {
		details, err = GetTitle(ctx, ent.ImdbID)
	}

	// 2. Если ImdbID не найден, пробуем поиск по оригинальному названию / названию
	if (details == nil || err != nil) && ent.OriginalTitle != "" {
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
