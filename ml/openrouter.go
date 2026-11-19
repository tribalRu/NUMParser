package ml

import (
	"NUMParser/config"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	OpenRouterKey   = ""
	OpenRouterModel = "google/gemma-4-31b-it:free"

	verifiedOpenRouterModels   []string
	verifiedOpenRouterModelsMu sync.Mutex
)

type openRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openRouterResponseFormat struct {
	Type string `json:"type"`
}

type openRouterRequest struct {
	Model          string                    `json:"model"`
	Messages       []openRouterMessage       `json:"messages"`
	Temperature    float64                   `json:"temperature"`
	ResponseFormat *openRouterResponseFormat `json:"response_format,omitempty"`
}

type openRouterResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

func initOpenRouter() {
	var err error
	OpenRouterKey, err = config.ReadConfigParser("OpenRouterKey")
	if err != nil || OpenRouterKey == "" {
		dir := filepath.Dir(os.Args[0])
		buf, err := os.ReadFile(filepath.Join(dir, "openrouter.key"))
		if err == nil && strings.TrimSpace(string(buf)) != "" {
			OpenRouterKey = strings.TrimSpace(string(buf))
		}
	}
	OpenRouterKey = cleanAPIKey(OpenRouterKey)

	configuredModel, _ := config.ReadConfigParser("OpenRouterModel")
	configuredModel = strings.TrimSpace(configuredModel)
	if configuredModel != "" {
		OpenRouterModel = configuredModel
	}
	if OpenRouterModel == "" || OpenRouterModel == "google/gemini-2.0-flash-exp:free" {
		OpenRouterModel = "google/gemma-4-31b-it:free"
	}

	if OpenRouterKey != "" {
		masked := OpenRouterKey
		if len(masked) > 8 {
			masked = masked[:4] + "..." + masked[len(masked)-4:]
		}
		log.Printf("[OpenRouter] Инициализирован с моделью %q, ключ: %s\n", OpenRouterModel, masked)
	}
}

func rankOpenRouterModel(id string) int {
	lower := strings.ToLower(id)
	switch {
	case strings.Contains(lower, "gemma-4"):
		return 1
	case strings.Contains(lower, "nemotron"):
		return 2
	case strings.Contains(lower, "ling"):
		return 3
	case strings.Contains(lower, "qwen"):
		return 4
	case strings.Contains(lower, "llama-3"):
		return 5
	case strings.Contains(lower, "mistral"):
		return 6
	default:
		return 10
	}
}

func fetchOpenRouterFreeModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		return nil, err
	}
	if OpenRouterKey != "" {
		req.Header.Set("Authorization", "Bearer "+OpenRouterKey)
	}
	req.Header.Set("User-Agent", "NUMParser/2.0")

	tr := &http.Transport{
		MaxIdleConns:        5,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	if config.UseProxy && config.ProxyHost != "" {
		if u, err := url.Parse(config.ProxyHost); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	httpClient := &http.Client{Transport: tr, Timeout: 15 * time.Second}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connection error: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var mResp struct {
		Data []struct {
			ID      string `json:"id"`
			Pricing struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &mResp); err != nil {
		return nil, fmt.Errorf("failed to decode models: %w", err)
	}

	var ids []string
	for _, m := range mResp.Data {
		lower := strings.ToLower(m.ID)
		if strings.Contains(lower, "embed") || strings.Contains(lower, "guard") || strings.Contains(lower, "moderation") || strings.Contains(lower, "audio") || strings.Contains(lower, "whisper") {
			continue
		}
		isFree := strings.HasSuffix(lower, ":free") || (m.Pricing.Prompt == "0" && m.Pricing.Completion == "0")
		if isFree {
			ids = append(ids, m.ID)
		}
	}

	sort.Slice(ids, func(i, j int) bool {
		return rankOpenRouterModel(ids[i]) < rankOpenRouterModel(ids[j])
	})

	return ids, nil
}

func callOpenRouterWithModel(ctx context.Context, model, systemPrompt, userPrompt string, jsonMode bool) (string, error) {
	var messages []openRouterMessage
	if systemPrompt != "" {
		messages = append(messages, openRouterMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	messages = append(messages, openRouterMessage{
		Role:    "user",
		Content: userPrompt,
	})

	reqBody := openRouterRequest{
		Model:       model,
		Messages:    messages,
		Temperature: 0.7,
	}
	if jsonMode {
		reqBody.ResponseFormat = &openRouterResponseFormat{Type: "json_object"}
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+OpenRouterKey)
	req.Header.Set("HTTP-Referer", "https://github.com/YouROK/NUMParser")
	req.Header.Set("X-Title", "NUMParser")

	tr := &http.Transport{
		MaxIdleConns:        5,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	if config.UseProxy && config.ProxyHost != "" {
		if u, err := url.Parse(config.ProxyHost); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	httpClient := &http.Client{Transport: tr, Timeout: 45 * time.Second}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("connection error: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var orResp openRouterResponse
		_ = json.Unmarshal(bodyBytes, &orResp)
		errMsg := string(bodyBytes)
		if orResp.Error != nil && orResp.Error.Message != "" {
			errMsg = orResp.Error.Message
		}

		// If jsonMode failed due to unsupported response_format, retry without it
		if jsonMode && (strings.Contains(errMsg, "response_format") || strings.Contains(errMsg, "json_object")) {
			log.Printf("[OpenRouter] Модель %s не поддерживает response_format json_object, повторяем без него...", model)
			return callOpenRouterWithModel(ctx, model, systemPrompt, userPrompt, false)
		}

		return "", fmt.Errorf("OpenRouter API error (status %d): %s", resp.StatusCode, errMsg)
	}

	var orResp openRouterResponse
	if err := json.Unmarshal(bodyBytes, &orResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if orResp.Error != nil && orResp.Error.Message != "" {
		return "", fmt.Errorf("OpenRouter error: %s (code %d)", orResp.Error.Message, orResp.Error.Code)
	}

	if len(orResp.Choices) == 0 || orResp.Choices[0].Message.Content == "" {
		return "", errors.New("OpenRouter returned empty content")
	}

	return orResp.Choices[0].Message.Content, nil
}

// CallOpenRouter executes chat completion via OpenRouter with automatic fallback candidates
func CallOpenRouter(ctx context.Context, systemPrompt, userPrompt string, jsonMode bool) (string, error) {
	if OpenRouterKey == "" {
		return "", errors.New("OpenRouter API key not configured")
	}

	// 1. Попытка с настроенной моделью
	modelToTry := OpenRouterModel
	content, err := callOpenRouterWithModel(ctx, modelToTry, systemPrompt, userPrompt, jsonMode)
	if err == nil {
		return content, nil
	}

	errMsg := err.Error()
	isNotFound := strings.Contains(errMsg, "no provider serves this model") || strings.Contains(errMsg, "is unavailable for free") || strings.Contains(errMsg, "status 404") || strings.Contains(errMsg, "not found")

	if isNotFound {
		log.Printf("[OpenRouter] Модель %q недоступна (%v), запрашиваем список бесплатных моделей...", modelToTry, err)

		verifiedOpenRouterModelsMu.Lock()
		if len(verifiedOpenRouterModels) == 0 {
			liveModels, mErr := fetchOpenRouterFreeModels(ctx)
			if mErr != nil {
				log.Printf("[OpenRouter] Не удалось обновить список моделей с сервера: %v\n", mErr)
			} else if len(liveModels) > 0 {
				log.Printf("[OpenRouter] Актуальные бесплатные модели OpenRouter: %v\n", liveModels)
				verifiedOpenRouterModels = liveModels
			}
		}
		modelsToTry := make([]string, len(verifiedOpenRouterModels))
		copy(modelsToTry, verifiedOpenRouterModels)
		verifiedOpenRouterModelsMu.Unlock()

		for _, m := range modelsToTry {
			if m == modelToTry {
				continue
			}
			log.Printf("[OpenRouter] Пробуем доступную бесплатную модель %q...\n", m)
			altContent, altErr := callOpenRouterWithModel(ctx, m, systemPrompt, userPrompt, jsonMode)
			if altErr == nil {
				log.Printf("[OpenRouter] Модель %q успешно ответила! Переключаем активную модель на %q\n", m, m)
				OpenRouterModel = m
				return altContent, nil
			}
		}
	}

	// 2. Статический список актуальных бесплатных моделей
	candidates := []string{
		"google/gemma-4-31b-it:free",
		"google/gemma-4-26b-a4b-it:free",
		"nvidia/nemotron-3.5-lightning:free",
		"inclusionai/ling-3.2-flash:free",
	}

	tried := make(map[string]bool)
	tried[modelToTry] = true
	var lastErr = err

	for _, m := range candidates {
		if m == "" || tried[m] {
			continue
		}
		tried[m] = true

		altContent, altErr := callOpenRouterWithModel(ctx, m, systemPrompt, userPrompt, jsonMode)
		if altErr == nil {
			log.Printf("[OpenRouter] Резервная модель %q успешно ответила! Переключаем активную модель на %q\n", m, m)
			OpenRouterModel = m
			return altContent, nil
		}
		lastErr = altErr
		log.Printf("[OpenRouter] Ошибка для модели %q: %v, пробуем следующую...\n", m, altErr)
	}

	return "", lastErr
}

func genCollectionOpenRouter(ctx context.Context, systemPrompt, userPrompt string) ([]*Collection, error) {
	rawJSON, err := CallOpenRouter(ctx, systemPrompt, userPrompt, true)
	if err != nil {
		return nil, err
	}
	return parseCollectionsJSON(rawJSON)
}

func getCollectionMoviesOpenRouter(ctx context.Context, systemPrompt, userPrompt string) ([]*MovieInfo, error) {
	content, err := CallOpenRouter(ctx, systemPrompt, userPrompt, false)
	if err != nil {
		return nil, err
	}
	movies := parseTitles(content)
	if len(movies) == 0 {
		return nil, errors.New("no movies parsed from OpenRouter response")
	}
	return movies, nil
}
