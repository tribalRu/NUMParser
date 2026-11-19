package ml

import (
	"NUMParser/config"
	"NUMParser/utils"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
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
	initOpenRouter()

	var configuredAI []string
	if GoogleAiKey != "" {
		configuredAI = append(configuredAI, "Google Gemini")
	}
	if GroqKey != "" {
		configuredAI = append(configuredAI, fmt.Sprintf("Groq (%s)", GroqModel))
	}
	if OpenRouterKey != "" {
		configuredAI = append(configuredAI, fmt.Sprintf("OpenRouter (%s)", OpenRouterModel))
	}

	if len(configuredAI) > 0 {
		log.Printf("AI configured (in priority order): %s\n", strings.Join(configuredAI, " -> "))
	} else {
		log.Println("Notice: no AI key configured (collections generation will use seasonal presets)")
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
		count = 15
	}
	if count > 18 {
		count = 18 // Оптимальный размер: предотвращает превышение лимитов токенов (OTPM) на бесплатных тарифах Groq и OpenRouter
	}

	season := GetSeasonContext(time.Now())

	recentContext := GetRecentReleasesContext(20)
	recentSection := ""
	if recentContext != "" {
		recentSection = fmt.Sprintf("\n\nСПИСОК СВЕЖИХ РЕЛИЗОВ С ТРЕКЕРА (2024–2026 гг.):\n%s\nИспользуй эти актуальные релизы для создания злободневных, трендовых коллекций (например: подборки вокруг этих новинок, франшизы, похожие фильмы, режиссеры новинок).", recentContext)
	}

	var prevTitles []string
	if CollsConfig != nil && len(CollsConfig.Collections) > 0 {
		for _, c := range CollsConfig.Collections {
			if c != nil && strings.TrimSpace(c.Title) != "" {
				prevTitles = append(prevTitles, fmt.Sprintf("%q", strings.TrimSpace(c.Title)))
			}
		}
	}
	antiRepeatSection := ""
	if len(prevTitles) > 0 {
		antiRepeatSection = fmt.Sprintf("\n\nКАТЕГОРИЧЕСКИЙ ЗАПРЕТ ПОВТОРОВ (СПИСОК ПРЕДЫДУЩИХ ПОДБОРОК):\nПользователь запросил ПЕРЕГЕНЕРАЦИЮ подборок. СТРОЖАЙШЕ ЗАПРЕЩЕНО повторять или перефразировать темы из предыдущего списка:\n[%s]\nКаждая из новых %d подборок ОБЯЗАНА иметь СОВЕРШЕННО НОВУЮ, оригинальную, неожиданную тему, которой не было в списке выше!", strings.Join(prevTitles, ", "), count)
	}

	themePool := []string{
		"Нео-нуар и ночные расследования в дождливых мегаполисах",
		"Идеальные ограбления, виртуозные аферы и карточные фокусы",
		"Космический ужас и психологическая изоляция на орбитальных станциях",
		"Роуд-муви: побеги, неожиданные попутчики и дороги, меняющие судьбы",
		"Судебные баталии, присяжные и напряженные юридические дуэли",
		"Камерные триллеры одного помещения и герметичные детективы",
		"Культовые боевики и фантастика эпохи VHS (80-е и 90-е)",
		"Азиатский триумф: корейские триллеры, японский нео-экшн, гонконгские боевики",
		"Скандинавский нуар: ледяное спокойствие и мрачные семейные секреты",
		"Фильмы-головоломки: игры разума, нелинейное время и ложные воспоминания",
		"Человек против стихии: выживание на грани жизни и смерти в экстремальных условиях",
		"Черные комедии, едкая сатира и циничный криминальный юмор",
		"Шпионские игры, двойные агенты и тайны холодной войны",
		"Гастрономические драмы и безумная страсть к высокой кухне",
		"Спортивные драмы, триумфы аутсайдеров и преодоление себя",
		"Мистические тайны заброшенных мест, сект и старинных особняков",
		"Киберпанк, биохакинг, аугментации и восстание искусственного интеллекта",
		"Шедевры европейского авторского кинематографа (Франция, Италия, Испания, Германия)",
		"Подводные бездны, субмарины и тайны океанских глубин",
		"Гангстерские саги, войны криминальных кланов и мафиозные хроники",
		"Временные петли, парадоксы путешествий во времени и мультивселенные",
		"Авиакатастрофы, воздушные дуэли и захваты самолетов",
		"Параноидальные политические триллеры и теории заговора",
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(themePool), func(i, j int) {
		themePool[i], themePool[j] = themePool[j], themePool[i]
	})
	focalCount := 5
	if focalCount > len(themePool) {
		focalCount = len(themePool)
	}
	focusSection := fmt.Sprintf("\n- Свежие фокусные кинематографические направления для фантазии: %s", strings.Join(themePool[:focalCount], "; "))

	systemPrompt := `Ты профессиональный русскоязычный кинокритик.
Твоя задача — создавать качественные тематические коллекции фильмов для зрителей.
КРИТИЧЕСКИЕ ТРЕБОВАНИЯ:
1. Все поля "Title" (название коллекции) и "Overview" (описание коллекции) ДОЛЖНЫ БЫТЬ СТРОГО НА РУССКОМ ЯЗЫКЕ!
2. Никакого английского текста в полях Title и Overview.
3. Отвечай СТРОГО валидным JSON-объектом формата: {"collections": [{"Title": "...", "Overview": "...", "Prompt": "..."}]}`

	userPrompt := fmt.Sprintf(`Сгенерируй ровно %d уникальных и разнообразных коллекций фильмов на эту неделю.
Текущий сезон: %s (%s). Праздники и контекст: %s.
Обязательно включи в подборку:
- Сезонные и праздничные темы с акцентом на: %s
- Современные трендовые темы: хиты последних лет, антиутопии, искусственный интеллект, триумф европейского/азиатского кино, фильмы с неожиданной развязкой
- Необычные жанровые коллекции: идеальные ограбления, камерные триллеры, выживание против стихии, черные комедии, роуд-муви, киберпанк
- Культовые классические темы: шедевры 80-90-х, авторские режиссеры, золотой фонд советского кино.%s%s%s

Формат ответа — СТРОГО валидный JSON-объект:
{
  "collections": [
    {
      "Title": "Название коллекции на русском",
      "Overview": "Краткое привлекательное описание коллекции на русском для зрителя",
      "Prompt": "Конкретное указание для поиска фильмов (жанры, темы, период)"
    }
  ]
}`, count, season.SeasonName, season.MonthName, season.Holidays, season.Hints, focusSection, recentSection, antiRepeatSection)

	// 1. Попытка через Google Gemini (собственный таймаут 30 сек)
	if GoogleAiKey != "" && !isGeminiQuotaBlocked() {
		geminiCtx, cancelGemini := context.WithTimeout(context.Background(), 30*time.Second)
		colls, err := genCollectionGemini(geminiCtx, userPrompt)
		cancelGemini()
		if err == nil && len(colls) > 0 {
			log.Printf("Successfully generated %d collections via Gemini\n", len(colls))
			return colls, nil
		}
		if isQuotaError(err) {
			markGeminiQuotaExceeded(12 * time.Hour)
		}
		log.Printf("Gemini collection generation failed (%v), falling back to Groq...\n", err)
	}

	// 2. Фолбек через Groq (собственный таймаут 45 сек)
	if GroqKey != "" {
		groqCtx, cancelGroq := context.WithTimeout(context.Background(), 45*time.Second)
		colls, err := genCollectionGroq(groqCtx, systemPrompt, userPrompt)
		cancelGroq()
		if err == nil && len(colls) > 0 {
			log.Printf("Successfully generated %d collections via Groq (%s)\n", len(colls), GroqModel)
			return colls, nil
		}
		log.Printf("Groq collection generation failed: %v\n", err)
	}

	// 3. Фолбек через OpenRouter (собственный независимый таймаут 45 сек)
	if OpenRouterKey != "" {
		orCtx, cancelOr := context.WithTimeout(context.Background(), 45*time.Second)
		colls, err := genCollectionOpenRouter(orCtx, systemPrompt, userPrompt)
		cancelOr()
		if err == nil && len(colls) > 0 {
			log.Printf("Successfully generated %d collections via OpenRouter (%s)\n", len(colls), OpenRouterModel)
			return colls, nil
		}
		log.Printf("OpenRouter collection generation failed: %v\n", err)
	}

	// 4. Фолбек на сезонную ротацию пресетов
	log.Println("All AI providers failed or not configured, using seasonal rotation of built-in presets")
	return rotateDefaultCollections(count, season), nil
}

var geminiCandidates = []string{
	"gemini-3.8-flash",
	"gemini-2.5-flash",
}

func generateWithGemini(ctx context.Context, client *genai.Client, contents []*genai.Content) (*genai.GenerateContentResponse, error) {
	var lastErr error
	for _, m := range geminiCandidates {
		resp, err := client.Models.GenerateContent(ctx, m, contents, nil)
		if err == nil && resp != nil && len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
			return resp, nil
		}
		if err != nil {
			lastErr = err
			if isQuotaError(err) {
				markGeminiQuotaExceeded(12 * time.Hour)
				return nil, err
			}
			errMsg := strings.ToLower(err.Error())
			// Прерываем перебор только при невалидном или заблокированном ключе (403)
			if strings.Contains(errMsg, "403") || strings.Contains(errMsg, "permission_denied") || strings.Contains(errMsg, "api_key_invalid") {
				return nil, err
			}
			log.Printf("[Gemini] Модель %s вернула ошибку (%v), пробуем следующую модель...", m, err)
		}
	}
	return nil, lastErr
}

func genCollectionGemini(ctx context.Context, prompt string) ([]*Collection, error) {
	client, err := getGeminiClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	contents := []*genai.Content{
		genai.NewContentFromText(prompt, genai.RoleUser),
	}

	resp, err := generateWithGemini(ctx, client, contents)
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

func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	s = strings.TrimSpace(s)

	startObj := strings.Index(s, "{")
	startArr := strings.Index(s, "[")
	start := -1
	if startObj != -1 && startArr != -1 {
		if startObj < startArr {
			start = startObj
		} else {
			start = startArr
		}
	} else if startObj != -1 {
		start = startObj
	} else if startArr != -1 {
		start = startArr
	}

	if start == -1 {
		return s
	}

	endObj := strings.LastIndex(s, "}")
	endArr := strings.LastIndex(s, "]")
	end := -1
	if endObj > endArr {
		end = endObj
	} else {
		end = endArr
	}

	if end > start {
		return s[start : end+1]
	}
	return s
}

func cleanAndFilterCollections(colls []*Collection) []*Collection {
	var result []*Collection
	seen := make(map[string]bool)
	for _, c := range colls {
		if c == nil {
			continue
		}
		c.Title = strings.TrimSpace(c.Title)
		c.Overview = strings.TrimSpace(c.Overview)
		c.Prompt = strings.TrimSpace(c.Prompt)
		if c.Title == "" {
			continue
		}
		key := utils.ClearStr(c.Title)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, c)
	}
	return result
}

func parseCollectionsJSON(text string) ([]*Collection, error) {
	cleaned := extractJSON(text)

	// Попытка 1: прямой массив []*Collection
	var collections []*Collection
	if err := json.Unmarshal([]byte(cleaned), &collections); err == nil && len(collections) > 0 {
		res := cleanAndFilterCollections(collections)
		if len(res) > 0 {
			return res, nil
		}
	}

	// Попытка 2: обёртка в объект {"collections": [...]}
	var wrapper struct {
		Collections []*Collection `json:"collections"`
	}
	if err := json.Unmarshal([]byte(cleaned), &wrapper); err == nil && len(wrapper.Collections) > 0 {
		res := cleanAndFilterCollections(wrapper.Collections)
		if len(res) > 0 {
			return res, nil
		}
	}

	// Попытка 3: обёртка в объект {"items": [...]} или {"data": [...]}
	var altWrapper struct {
		Items []*Collection `json:"items"`
		Data  []*Collection `json:"data"`
	}
	if err := json.Unmarshal([]byte(cleaned), &altWrapper); err == nil {
		if len(altWrapper.Items) > 0 {
			res := cleanAndFilterCollections(altWrapper.Items)
			if len(res) > 0 {
				return res, nil
			}
		}
		if len(altWrapper.Data) > 0 {
			res := cleanAndFilterCollections(altWrapper.Data)
			if len(res) > 0 {
				return res, nil
			}
		}
	}

	return nil, fmt.Errorf("failed to parse JSON from AI response: %s", text)
}

// GetCollectionMovies fetches movie suggestions for a specific collection.
// Uses Gemini as primary and Groq as fallback, with strict Russian support.
func GetCollectionMovies(collection *Collection) ([]*MovieInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	systemPrompt := `Ты профессиональный русскоязычный кинокритик.
Твоя задача — составить подборку фильмов, СТРОГО и БЕЗУПРЕЧНО соответствующих заданной теме коллекции.

ОБЯЗАТЕЛЬНЫЕ ПРАВИЛА:
1. СТРОЖАЙШЕЕ СООТВЕТСТВИЕ ТЕМЕ, ЖАНРУ И ГЕОГРАФИИ:
   - Если тема азиатская (Азия, Япония, Южная Корея, Китай, Гонконг) — ВСЕ фильмы ОБЯЗАНЫ быть исключительно производства стран Азии! Категорически запрещено включать фильмы США, СССР, России или Европы!
   - Если тема французская/европейская/советская/скандинавская — фильмы должны быть СТРОГО из этих стран.
   - Если тема криминальная (мафия, ограбления, детективы) — включай ТОЛЬКО криминальные драмы и триллеры. Никакой мистики или посторонних жанров!
   - Каждый фильм обязан на 100% точно отражать тему подборки. Никаких случайных фильмов «не в тему»!
2. ВЫБИРАЙ ТОЛЬКО ИЗВЕСТНЫЕ ХИТЫ И КУЛЬТОВУЮ КЛАССИКУ:
   - Выбирай популярные, высокооцененные и любимые зрителями фильмы с широким мировым прокатом, которые гарантированно есть на торрент-трекерах.
   - Избегай малоизвестного артхауса, любительских или труднодоступных фильмов.
3. ФОРМАТ ОТВЕТА: строго список названий с годом в скобках, разделенных ТОЛЬКО точкой с запятой:
   Название1 (Год1); Название2 (Год2); Название3 (Год3); ...
4. Названия фильмов пиши на русском языке (официальное прокатное название). Год выпуска — в круглых скобках.
5. Ровно 20–25 фильмов. Только полнометражные художественные фильмы. Короткометражки и сериалы не включать. Без повторов.`

	userPrompt := fmt.Sprintf("Тема коллекции: %s\nОписание: %s\nТребования: %s\n\nСоставь список из 20–25 лучших, самых известных и высокооцененных фильмов, СТРОГО соответствующих этой теме.",
		collection.Title, collection.Overview, collection.Prompt)

	// 1. Попытка через Google Gemini
	if GoogleAiKey != "" && !isGeminiQuotaBlocked() {
		movies, err := getCollectionMoviesGemini(ctx, systemPrompt+"\n\n"+userPrompt)
		if err == nil && len(movies) > 0 {
			return movies, nil
		}
		if isQuotaError(err) {
			markGeminiQuotaExceeded(12 * time.Hour)
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

	// 3. Фолбек через OpenRouter
	if OpenRouterKey != "" {
		movies, err := getCollectionMoviesOpenRouter(ctx, systemPrompt, userPrompt)
		if err == nil && len(movies) > 0 {
			return movies, nil
		}
		log.Printf("OpenRouter movies failed for %q: %v\n", collection.Title, err)
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

	resp, err := generateWithGemini(ctx, client, contents)
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
	seen := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 2 {
			title := cleanTitle(match[1])
			year := match[2]
			cleanKey := utils.ClearStr(title)
			if cleanKey == "" || seen[cleanKey] {
				continue
			}
			seen[cleanKey] = true
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

	// Seasonal injected topic with randomized dynamic title
	seasonalTitles := map[string][]string{
		"Зима": {
			"Зимняя сказка: Уютное семейное кино",
			"Зимний вечер: Шедевры у камина",
			"Новогоднее настроение и волшебство",
			"Ледяное безмолвие: Захватывающие зимние драмы",
			"Зимний марафон культового кино",
		},
		"Весна": {
			"Весеннее пробуждение: Романтика и светлые чувства",
			"Весенний драйв: Яркие приключения",
			"Весенний киносезон: Свежий взгляд и новинки",
			"Майские праздники: Большое приключенческое кино",
		},
		"Лето": {
			"Летний зной: Жаркие блокбастеры и боевики",
			"Летнее солнцестояние: Солнечные комедии для отдыха",
			"Летние приключения и дальние неизведанные страны",
			"Жаркие ночи: Остросюжетные триллеры",
		},
		"Осень": {
			"Осенние размышления: Глубокие и сильные драмы",
			"Осенний уют: Тёплые истории под пледом",
			"Осенний нуар: Меланхолия, дождь и городские тайны",
			"Осенняя мистика и туманные загадки",
			"Золотая осень: Кинематографический блюз",
		},
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	seasonTitle := fmt.Sprintf("%s: Киносезон", season.SeasonName)
	if titles, ok := seasonalTitles[season.SeasonName]; ok && len(titles) > 0 {
		seasonTitle = titles[r.Intn(len(titles))]
	}

	seasonalColl := &Collection{
		Title:    seasonTitle,
		Overview: fmt.Sprintf("Атмосферная подборка для сезона %s (%s). %s", season.SeasonName, season.MonthName, season.Holidays),
		Prompt:   season.Hints,
	}
	res = append(res, seasonalColl)

	// Track previously shown titles so rotation prioritizes fresh ones
	prevTitles := make(map[string]bool)
	if CollsConfig != nil {
		for _, c := range CollsConfig.Collections {
			if c != nil {
				prevTitles[utils.ClearStr(c.Title)] = true
			}
		}
	}

	indices := r.Perm(len(defcolls))

	// First pass: add defcolls that were NOT in the previous collection set
	for _, idx := range indices {
		if len(res) >= count {
			break
		}
		c := defcolls[idx]
		if !prevTitles[utils.ClearStr(c.Title)] {
			cCopy := *c
			res = append(res, &cCopy)
		}
	}

	// Second pass: if we still need more collections to satisfy count, add from remaining
	if len(res) < count {
		for _, idx := range indices {
			if len(res) >= count {
				break
			}
			c := defcolls[idx]
			alreadyInRes := false
			for _, rItem := range res {
				if rItem.Title == c.Title {
					alreadyInRes = true
					break
				}
			}
			if !alreadyInRes {
				cCopy := *c
				res = append(res, &cCopy)
			}
		}
	}

	return res
}
