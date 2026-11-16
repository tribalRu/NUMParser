package ml

import (
	"context"
	"testing"
	"time"
)

func TestParseTitles(t *testing.T) {
	input := "Inception (2010); Interstellar (2014); The Dark Knight (2008); Брат (1997);"
	movies := parseTitles(input)
	if len(movies) != 4 {
		t.Fatalf("expected 4 movies, got %d", len(movies))
	}
	if movies[0].Title != "Inception" || movies[0].Year != "2010" {
		t.Errorf("unexpected first movie: %+v", movies[0])
	}
	if movies[3].Title != "Брат" || movies[3].Year != "1997" {
		t.Errorf("unexpected fourth movie: %+v", movies[3])
	}
}

func TestGetSeasonContext(t *testing.T) {
	// Winter - late December (New Year)
	winterTime := time.Date(2026, time.December, 25, 12, 0, 0, 0, time.UTC)
	sWinter := GetSeasonContext(winterTime)
	if sWinter.SeasonName != "Зима" {
		t.Errorf("expected Зима, got %s", sWinter.SeasonName)
	}

	// Spring - April (Cosmonautics)
	springTime := time.Date(2026, time.April, 12, 12, 0, 0, 0, time.UTC)
	sSpring := GetSeasonContext(springTime)
	if sSpring.SeasonName != "Весна" {
		t.Errorf("expected Весна, got %s", sSpring.SeasonName)
	}

	// May - Victory day
	mayTime := time.Date(2026, time.May, 9, 12, 0, 0, 0, time.UTC)
	sMay := GetSeasonContext(mayTime)
	if sMay.SeasonName != "Весна" {
		t.Errorf("expected Весна, got %s", sMay.SeasonName)
	}

	// Autumn - late October (Halloween)
	autumnTime := time.Date(2026, time.October, 31, 12, 0, 0, 0, time.UTC)
	sAutumn := GetSeasonContext(autumnTime)
	if sAutumn.SeasonName != "Осень" {
		t.Errorf("expected Осень, got %s", sAutumn.SeasonName)
	}
}

func TestParseCollectionsJSON(t *testing.T) {
	// 1. Direct array JSON
	directJSON := `[
		{"Title": "Космическая одиссея", "Overview": "Лучшая фантастика о космосе", "Prompt": "Фильмы про космос"},
		{"Title": "Ночной нуар", "Overview": "Атмосферные дождливые детективы", "Prompt": "Неонуар и детективы"}
	]`
	colls, err := parseCollectionsJSON(directJSON)
	if err != nil {
		t.Fatalf("failed to parse direct JSON: %v", err)
	}
	if len(colls) != 2 || colls[0].Title != "Космическая одиссея" {
		t.Errorf("unexpected parsed collections: %+v", colls)
	}

	// 2. Markdown fenced JSON
	fencedJSON := "```json\n" + directJSON + "\n```"
	collsFenced, err := parseCollectionsJSON(fencedJSON)
	if err != nil {
		t.Fatalf("failed to parse fenced JSON: %v", err)
	}
	if len(collsFenced) != 2 {
		t.Errorf("unexpected parsed count: %d", len(collsFenced))
	}

	// 3. Object wrapper {"collections": [...]}
	wrapperJSON := `{"collections": [
		{"Title": "Советская классика", "Overview": "Любимые комедии Гайдая", "Prompt": "Комедии СССР"}
	]}`
	collsWrapper, err := parseCollectionsJSON(wrapperJSON)
	if err != nil {
		t.Fatalf("failed to parse wrapper JSON: %v", err)
	}
	if len(collsWrapper) != 1 || collsWrapper[0].Title != "Советская классика" {
		t.Errorf("unexpected wrapper collections: %+v", collsWrapper)
	}
}

func TestRotateDefaultCollections(t *testing.T) {
	season := GetSeasonContext(time.Now())
	colls := rotateDefaultCollections(15, season)
	if len(colls) != 15 {
		t.Fatalf("expected 15 collections, got %d", len(colls))
	}
	// First collection should be seasonal
	if colls[0].Title == "" || colls[0].Overview == "" {
		t.Errorf("expected non-empty seasonal collection: %+v", colls[0])
	}
}

func TestGroqEmptyKey(t *testing.T) {
	oldKey := GroqKey
	GroqKey = ""
	defer func() { GroqKey = oldKey }()

	_, err := CallGroq(context.Background(), "sys", "usr", false)
	if err == nil {
		t.Fatal("expected error when GroqKey is empty")
	}
}

func TestGetRecentReleasesContext(t *testing.T) {
	ctx := GetRecentReleasesContext(50)
	// When no DB initialized, it returns empty string without crashing
	_ = ctx
}

func TestRankModel(t *testing.T) {
	if rankModel("openai/gpt-oss-20b") >= rankModel("openai/gpt-oss-120b") {
		t.Errorf("gpt-oss-20b should have higher priority than gpt-oss-120b")
	}
	if rankModel("qwen/qwen3.8-27b") >= rankModel("allam-2-7b") {
		t.Errorf("qwen should have higher priority than allam")
	}
	if rankModel("allam-2-7b") <= rankModel("openai/gpt-oss-20b") {
		t.Errorf("allam should have lower priority than gpt-oss-20b")
	}
}

