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
	GroqKey   = ""
	GroqModel = "openai/gpt-oss-20b"

	verifiedModels   []string
	verifiedModelsMu sync.Mutex
)

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqResponseFormat struct {
	Type string `json:"type"`
}

type groqRequest struct {
	Model          string              `json:"model"`
	Messages       []groqMessage       `json:"messages"`
	Temperature    float64             `json:"temperature"`
	ResponseFormat *groqResponseFormat `json:"response_format,omitempty"`
}

type groqResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

func cleanAPIKey(k string) string {
	k = strings.TrimSpace(k)
	k = strings.Trim(k, `"'`)
	k = strings.TrimPrefix(k, "Bearer ")
	k = strings.TrimPrefix(k, "bearer ")
	return strings.TrimSpace(k)
}

func initGroq() {
	var err error
	GroqKey, err = config.ReadConfigParser("GroqKey")
	if err != nil || GroqKey == "" {
		dir := filepath.Dir(os.Args[0])
		buf, err := os.ReadFile(filepath.Join(dir, "groq.key"))
		if err == nil && strings.TrimSpace(string(buf)) != "" {
			GroqKey = strings.TrimSpace(string(buf))
		}
	}
	GroqKey = cleanAPIKey(GroqKey)
	if GroqKey != "" {
		if !strings.HasPrefix(GroqKey, "gsk_") {
			prefix := GroqKey
			if len(prefix) > 8 {
				prefix = prefix[:8] + "..."
			}
			log.Printf("[Groq] ВНИМАНИЕ: Ключ Groq API обычно начинается с 'gsk_'. Проверьте ключ на console.groq.com/keys (текущий префикс: %q)\n", prefix)
		}
	}
	model, err := config.ReadConfigParser("GroqModel")
	if err == nil && strings.TrimSpace(model) != "" {
		GroqModel = strings.TrimSpace(model)
	}
	// Migrate legacy/inaccessible model names
	if GroqModel == "" || GroqModel == "llama-3.1-8b-instant" || GroqModel == "llama-3.3-70b-versatile" || GroqModel == "llama3-70b-8192" || GroqModel == "mixtral-8x7b-32768" || GroqModel == "allam-2-7b" {
		GroqModel = "qwen/qwen3.8-27b"
	}
}

func rankModel(id string) int {
	lower := strings.ToLower(id)
	switch {
	case strings.Contains(lower, "qwen3.8-27b") || strings.Contains(lower, "qwen"):
		return 1
	case strings.Contains(lower, "gpt-oss-120b"):
		return 2
	case strings.Contains(lower, "gpt-oss-20b"):
		return 3
	case strings.Contains(lower, "llama-3.3-70b"):
		return 4
	case strings.Contains(lower, "llama-3.1-8b"):
		return 5
	case strings.Contains(lower, "gemma2-9b") || strings.Contains(lower, "gemma-2-9b"):
		return 6
	case strings.Contains(lower, "mixtral-8x7b"):
		return 7
	case strings.Contains(lower, "allam") || strings.Contains(lower, "whisper") || strings.Contains(lower, "guard"):
		return 999
	default:
		return 15
	}
}

// fetchGroqModels queries the Groq API for the list of models accessible to the current key
func fetchGroqModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.groq.com/openai/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+GroqKey)
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
			ID     string `json:"id"`
			Active bool   `json:"active"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &mResp); err != nil {
		return nil, fmt.Errorf("failed to decode models: %w", err)
	}

	var ids []string
	for _, m := range mResp.Data {
		lower := strings.ToLower(m.ID)
		if strings.Contains(lower, "whisper") || strings.Contains(lower, "guard") || strings.Contains(lower, "safeguard") || strings.Contains(lower, "allam") || strings.Contains(lower, "orpheus") || strings.Contains(lower, "canopylabs") {
			continue
		}
		ids = append(ids, m.ID)
	}

	sort.Slice(ids, func(i, j int) bool {
		return rankModel(ids[i]) < rankModel(ids[j])
	})

	return ids, nil
}

// CallGroq sends a chat completion request to the Groq API (OpenAI-compatible)
// with automatic fallback to alternate models if the configured model is unavailable (e.g. 404).
func CallGroq(ctx context.Context, systemPrompt, userPrompt string, jsonMode bool) (string, error) {
	if GroqKey == "" {
		return "", errors.New("groq API key not configured")
	}

	modelToTry := GroqModel
	if modelToTry == "" || modelToTry == "llama-3.3-70b-versatile" || modelToTry == "allam-2-7b" {
		modelToTry = "qwen/qwen3.8-27b"
	}
	content, err := callGroqWithModel(ctx, modelToTry, systemPrompt, userPrompt, jsonMode)
	if err == nil {
		return content, nil
	}

	log.Printf("[Groq] Модель %q вернула ошибку (%v), пробуем другие модели аккаунта...", modelToTry, err)

	// 1. Query the actual list of models allowed for this key
	verifiedModelsMu.Lock()
	if len(verifiedModels) == 0 {
		models, mErr := fetchGroqModels(ctx)
		if mErr != nil {
			log.Printf("[Groq] Не удалось запросить список доступных моделей с сервера Groq: %v\n", mErr)
		} else {
			log.Printf("[Groq] Доступные модели вашего аккаунта (по приоритету): %v\n", models)
			verifiedModels = models
		}
	}
	modelsToTry := make([]string, len(verifiedModels))
	copy(modelsToTry, verifiedModels)
	verifiedModelsMu.Unlock()

	for _, m := range modelsToTry {
		if m == modelToTry {
			continue
		}
		log.Printf("[Groq] Пробуем доступную модель аккаунта %q...\n", m)
		altContent, altErr := callGroqWithModel(ctx, m, systemPrompt, userPrompt, jsonMode)
		if altErr == nil {
			log.Printf("[Groq] Модель %q успешно ответила! Переключаем активную модель на %q\n", m, m)
			GroqModel = m
			return altContent, nil
		}
	}

	// 2. Fallback to static list of candidates
	candidates := []string{
		"qwen/qwen3.8-27b",
		"openai/gpt-oss-120b",
		"openai/gpt-oss-20b",
		"llama-3.3-70b-versatile",
		"llama-3.1-8b-instant",
	}
	for _, altModel := range candidates {
		if altModel == modelToTry {
			continue
		}
		log.Printf("[Groq] Пробуем резервную модель %q...\n", altModel)
		altContent, altErr := callGroqWithModel(ctx, altModel, systemPrompt, userPrompt, jsonMode)
		if altErr == nil {
			log.Printf("[Groq] Резервная модель %q успешно ответила! Переключаем активную модель на %q\n", altModel, altModel)
			GroqModel = altModel
			return altContent, nil
		}
	}

	return "", err
}

func callGroqWithModel(ctx context.Context, model string, systemPrompt, userPrompt string, jsonMode bool) (string, error) {
	messages := []groqMessage{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: userPrompt,
		},
	}

	reqBody := groqRequest{
		Model:       model,
		Messages:    messages,
		Temperature: 0.7,
	}
	if jsonMode {
		reqBody.ResponseFormat = &groqResponseFormat{Type: "json_object"}
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal groq request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewReader(jsonBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create groq request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+GroqKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NUMParser/2.0")

	// Set up HTTP transport (with proxy if configured)
	tr := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	if config.UseProxy && config.ProxyHost != "" {
		if u, err := url.Parse(config.ProxyHost); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	httpClient := &http.Client{
		Transport: tr,
		Timeout:   45 * time.Second,
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("groq network error: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read groq response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var gErr groqResponse
		_ = json.Unmarshal(bodyBytes, &gErr)
		errMsg := string(bodyBytes)
		if gErr.Error != nil && gErr.Error.Message != "" {
			errMsg = gErr.Error.Message
		}

		// If jsonMode failed due to unsupported response_format or 400 Bad Request, retry without response_format
		if jsonMode && (resp.StatusCode == http.StatusBadRequest || strings.Contains(errMsg, "response_format") || strings.Contains(errMsg, "json_object")) {
			log.Printf("[Groq] Модель %s не поддерживает response_format json_object, повторяем без него...", model)
			return callGroqWithModel(ctx, model, systemPrompt, userPrompt, false)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			log.Printf("[Groq] Rate limit (429) для %s, ждем 5 сек...", model)
			time.Sleep(5 * time.Second)
		}

		return "", fmt.Errorf("groq API error (status %d): %s", resp.StatusCode, errMsg)
	}

	var gResp groqResponse
	if err := json.Unmarshal(bodyBytes, &gResp); err != nil {
		return "", fmt.Errorf("failed to decode groq response: %w", err)
	}

	if len(gResp.Choices) == 0 || gResp.Choices[0].Message.Content == "" {
		return "", errors.New("groq returned empty content")
	}

	log.Printf("Groq responded successfully (model: %s)\n", model)
	return gResp.Choices[0].Message.Content, nil
}
