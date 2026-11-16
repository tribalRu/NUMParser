package ml

import (
	"NUMParser/config"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"
)

var (
	titleRegex  = regexp.MustCompile(`(.+?)\s*\(([12]\d{3})\);?`)
	GoogleAiKey = ""

	geminiClient   *genai.Client
	geminiClientMu sync.Mutex

	geminiQuotaBlockedUntil time.Time
	geminiQuotaMu           sync.RWMutex
)

func isGeminiQuotaBlocked() bool {
	geminiQuotaMu.RLock()
	defer geminiQuotaMu.RUnlock()
	return time.Now().Before(geminiQuotaBlockedUntil)
}

func markGeminiQuotaExceeded(duration time.Duration) {
	geminiQuotaMu.Lock()
	defer geminiQuotaMu.Unlock()
	geminiQuotaBlockedUntil = time.Now().Add(duration)
	log.Printf("[AI] Квота Google Gemini исчерпана (429 Quota Exceeded). Прямое переключение на Groq на %v без ожидания\n", duration)
}

func isQuotaError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "429") || strings.Contains(s, "quota") || strings.Contains(s, "resourceexhausted")
}

func getGeminiClient() (*genai.Client, error) {
	geminiClientMu.Lock()
	defer geminiClientMu.Unlock()
	if geminiClient != nil {
		return geminiClient, nil
	}
	c, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  GoogleAiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}
	geminiClient = c
	return geminiClient, nil
}

func Init() {
	log.Println("Init ml")

	var err error
	GoogleAiKey, err = config.ReadConfigParser("AigKey")
	if err != nil || GoogleAiKey == "" {
		dir := filepath.Dir(os.Args[0])
		buf, err := os.ReadFile(filepath.Join(dir, "aig.key"))
		if err == nil && strings.TrimSpace(string(buf)) != "" {
			GoogleAiKey = strings.TrimSpace(string(buf))
		}
	}

	initGroq()

	if GoogleAiKey != "" && GroqKey != "" {
		log.Printf("AI configured: Google Gemini (primary), Groq (%s, fallback)\n", GroqModel)
	} else if GoogleAiKey != "" {
		log.Println("AI configured: Google Gemini (primary, no fallback)")
	} else if GroqKey != "" {
		log.Printf("AI configured: Groq (%s, primary)\n", GroqModel)
	} else {
		log.Println("Notice: neither Google AI nor Groq key configured (collections generation will use seasonal presets)")
	}

	LoadConfig()

	if CollsConfig == nil {
		CollsConfig = &CollectionsConfig{
			Collections: nil,
			LastUpdated: time.Now().Add(-time.Hour * 200),
		}
	}

	if len(CollsConfig.Collections) == 0 && len(defcolls) > 0 {
		CollsConfig.Collections = make([]*Collection, len(defcolls))
		copy(CollsConfig.Collections, defcolls)
		SaveConfig()
	}
}

// GenCollection generates fresh collections with seasonal, holiday and trending mix.
// Uses Gemini as primary and Groq as fallback.
func GenCollection(count int) ([]*Collection, error) {
	if count <= 0 {
		count = 25
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	season := GetSeasonContext(time.Now())

	recentContext := GetRecentReleasesContext(60)
	recentSection := ""
	if recentContext != "" {
		recentSection = fmt.Sprintf("\n\nСПИСОК СВЕЖИХ РЕЛИЗОВ С ТРЕКЕРА (2024–2026 гг.):\n%s\nИспользуй эти актуальные релизы для создания злободневных, трендовых коллекций (например: подборки вокруг этих новинок, франшизы, похожие фильмы, режиссеры новинок).", recentContext)
	}

	systemPrompt := `Ты профессиональный русскоязычный кинокритик.
Твоя задача — создавать качественные тематические коллекции фильмов для зрителей.
КРИТИЧЕСКИЕ ТРЕБОВАНИЯ:
1. Все поля "Title" (название коллекции) и "Overview" (описание коллекции) ДОЛЖНЫ БЫТЬ СТРОГО НА РУССКОМ ЯЗЫКЕ!
2. Никакого английского текста в полях Title и Overview.
3. Отвечай СТРОГО валидным JSON-кодом (массив объектов с ключами "Title", "Overview", "Prompt").`

	userPrompt := fmt.Sprintf(`Сгенерируй ровно %d уникальных и разнообразных коллекций фильмов на эту неделю.
Текущий сезон: %s (%s). Праздники и контекст: %s.
Обязательно включи в подборку:
- 4-5 сезонных и праздничных тем с акцентом на: %s
- 6-7 современных трендовых тем на основе свежих релизов: хиты последних лет, антиутопии, искусственный интеллект, триумф европейского/азиатского кино, фильмы с неожиданной развязкой
- 8-9 необычных жанровых коллекций: идеальные ограбления, камерные триллеры, выживание против стихии, черные комедии, роуд-муви, киберпанк
- 5-6 культовых классических тем: шедевры 80-90-х, авторские режиссеры, золотой фонд советского кино.%s

Формат ответа — СТРОГО JSON массив объектов:
[
  {
    "Title": "Название коллекции на русском",
    "Overview": "Краткое привлекательное описание коллекции на русском для зрителя",
    "Prompt": "Конкретное указание для поиска фильмов (жанры, темы, период)"
  }
]`, count, season.SeasonName, season.MonthName, season.Holidays, season.Hints, recentSection)

	// 1. Попытка через Google Gemini
	if GoogleAiKey != "" && !isGeminiQuotaBlocked() {
		colls, err := genCollectionGemini(ctx, userPrompt)
		if err == nil && len(colls) > 0 {
			log.Printf("Successfully generated %d collections via Gemini\n", len(colls))
			return colls, nil
		}
		if isQuotaError(err) {
			markGeminiQuotaExceeded(15 * time.Minute)
		}
		log.Printf("Gemini collection generation failed (%v), falling back to Groq...\n", err)
	}

	// 2. Фолбек через Groq
	if GroqKey != "" {
		colls, err := genCollectionGroq(ctx, systemPrompt, userPrompt)
		if err == nil && len(colls) > 0 {
			log.Printf("Successfully generated %d collections via Groq (%s)\n", len(colls), GroqModel)
			return colls, nil
		}
		log.Printf("Groq collection generation failed: %v\n", err)
	}

	// 3. Фолбек на сезонную ротацию пресетов
	log.Println("Both AI providers failed or not configured, using seasonal rotation of built-in presets")
	return rotateDefaultCollections(count, season), nil
}

func genCollectionGemini(ctx context.Context, prompt string) ([]*Collection, error) {
	client, err := getGeminiClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	contents := []*genai.Content{
		genai.NewContentFromText(prompt, genai.RoleUser),
	}

	resp, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash", contents, nil)
	if err != nil {
		return nil, fmt.Errorf("Gemini API error: %w", err)
	}

	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return nil, fmt.Errorf("Gemini returned empty response")
	}

	var sb strings.Builder
	for _, cand := range resp.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				sb.WriteString(string(part.Text))
			}
		}
	}

	return parseCollectionsJSON(sb.String())
}

func genCollectionGroq(ctx context.Context, systemPrompt, userPrompt string) ([]*Collection, error) {
	rawJSON, err := CallGroq(ctx, systemPrompt, userPrompt, true)
	if err != nil {
		return nil, err
	}
	return parseCollectionsJSON(rawJSON)
}

func parseCollectionsJSON(text string) ([]*Collection, error) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
	}
	if strings.HasSuffix(text, "```") {
		text = strings.TrimSuffix(text, "```")
	}
	text = strings.TrimSpace(text)

	// Попытка 1: прямой массив []*Collection
	var collections []*Collection
	if err := json.Unmarshal([]byte(text), &collections); err == nil && len(collections) > 0 {
		return collections, nil
	}

	// Попытка 2: обёртка в объект {"collections": [...]}
	var wrapper struct {
		Collections []*Collection `json:"collections"`
	}
	if err := json.Unmarshal([]byte(text), &wrapper); err == nil && len(wrapper.Collections) > 0 {
		return wrapper.Collections, nil
	}

	return nil, fmt.Errorf("failed to parse JSON from AI response: %s", text)
}

// GetCollectionMovies fetches movie suggestions for a specific collection.
// Uses Gemini as primary and Groq as fallback, with strict Russian support.
func GetCollectionMovies(collection *Collection) ([]*MovieInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	systemPrompt := `Ты профессиональный русскоязычный кинокритик. Дай подборку фильмов. 
Ответ должен быть строго в виде списка названий с годом выпуска, разделенных ТОЛЬКО точкой с запятой.
Правила:
1. Формат строго: Title1 (Year1); Title2 (Year2); Title3 (Year3); ...
2. Указывай год выпуска в скобках.
3. Используй только реально существующие полнометражные художественные фильмы.
4. Кино производства СССР или России: названия ОБЯЗАТЕЛЬНО на русском языке!
5. Зарубежные фильмы: используй официальное русское прокатное название или оригинальное.
6. Короткометражки и сериалы не включать.
7. Не более 25 фильмов в ответе, строго без повторов.`

	recentContext := GetRecentReleasesContext(35)
	recentSection := ""
	if recentContext != "" {
		recentSection = fmt.Sprintf("\nВот актуальные кинорелизы 2024–2026 годов с нашего трекера: [%s]. Если какие-то из них подходят под тему подборки — ОБЯЗАТЕЛЬНО включи их в ответ вместе с классикой жанра.", recentContext)
	}

	userPrompt := fmt.Sprintf("Коллекция: %s\nОписание: %s\nПромпт: %s%s\nДай список лучших фильмов для этой коллекции.",
		collection.Title, collection.Overview, collection.Prompt, recentSection)

	// 1. Попытка через Google Gemini
	if GoogleAiKey != "" && !isGeminiQuotaBlocked() {
		movies, err := getCollectionMoviesGemini(ctx, systemPrompt+"\n"+userPrompt)
		if err == nil && len(movies) > 0 {
			return movies, nil
		}
		if isQuotaError(err) {
			markGeminiQuotaExceeded(15 * time.Minute)
		}
		log.Printf("Gemini movies failed for %q (%v), falling back to Groq...\n", collection.Title, err)
	}

	// 2. Фолбек через Groq
	if GroqKey != "" {
		movies, err := getCollectionMoviesGroq(ctx, systemPrompt, userPrompt)
		if err == nil && len(movies) > 0 {
			return movies, nil
		}
		log.Printf("Groq movies failed for %q: %v\n", collection.Title, err)
	}

	return nil, fmt.Errorf("all AI providers failed for collection %q", collection.Title)
}

func getCollectionMoviesGemini(ctx context.Context, prompt string) ([]*MovieInfo, error) {
	client, err := getGeminiClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	contents := []*genai.Content{
		genai.NewContentFromText(prompt, genai.RoleUser),
	}

	resp, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash", contents, nil)
	if err != nil {
		return nil, fmt.Errorf("Gemini API error: %w", err)
	}

	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return nil, fmt.Errorf("Gemini returned empty response")
	}

	var sb strings.Builder
	for _, cand := range resp.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				sb.WriteString(string(part.Text))
			}
		}
	}

	movies := parseTitles(sb.String())
	if len(movies) == 0 {
		return nil, fmt.Errorf("no movies parsed from response: %s", sb.String())
	}
	return movies, nil
}

func getCollectionMoviesGroq(ctx context.Context, systemPrompt, userPrompt string) ([]*MovieInfo, error) {
	resp, err := CallGroq(ctx, systemPrompt, userPrompt, false)
	if err != nil {
		return nil, err
	}
	movies := parseTitles(resp)
	if len(movies) == 0 {
		return nil, fmt.Errorf("no movies parsed from groq response: %s", resp)
	}
	return movies, nil
}

func parseTitles(response string) []*MovieInfo {
	matches := titleRegex.FindAllStringSubmatch(response, -1)
	var movies []*MovieInfo
	for _, match := range matches {
		if len(match) > 2 {
			title := cleanTitle(match[1])
			year := match[2]
			movies = append(movies, &MovieInfo{Title: title, Year: year})
		}
	}
	return movies
}

func cleanTitle(title string) string {
	title = strings.TrimSpace(title)
	return strings.Trim(title, `"'`)
}

// rotateDefaultCollections builds a curated set of collections from presets with seasonal injection
func rotateDefaultCollections(count int, season SeasonContext) []*Collection {
	if count <= 0 {
		count = 25
	}
	var res []*Collection

	// Seasonal injected topic
	seasonalColl := &Collection{
		Title:    fmt.Sprintf("%s: Киносезон", season.SeasonName),
		Overview: fmt.Sprintf("Атмосферная подборка для сезона %s (%s). %s", season.SeasonName, season.MonthName, season.Holidays),
		Prompt:   season.Hints,
	}
	res = append(res, seasonalColl)

	// Rotate from defcolls based on current week number
	weekNum := int(time.Now().Unix() / (7 * 24 * 3600))
	offset := (weekNum * 7) % len(defcolls)

	for i := 0; i < len(defcolls) && len(res) < count; i++ {
		idx := (offset + i) % len(defcolls)
		c := *defcolls[idx]
		res = append(res, &c)
	}

	return res
}
