package web

import (
	"NUMParser/config"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestWebRoutes(t *testing.T) {
	router := setupRouter()

	// Test GET /
	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET / returned status %d, expected %d", w.Code, http.StatusOK)
	}

	// Test GET /settings
	req, _ = http.NewRequest("GET", "/settings", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /settings returned status %d, expected %d", w.Code, http.StatusOK)
	}

	// Test GET /search without query -> 400
	req, _ = http.NewRequest("GET", "/search", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("GET /search without query returned status %d, expected %d", w.Code, http.StatusBadRequest)
	}

	// Test GET /search with query -> 200
	req, _ = http.NewRequest("GET", "/search?query=test", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /search?query=test returned status %d, expected %d", w.Code, http.StatusOK)
	}

	// Test GET /api/settings -> 200
	req, _ = http.NewRequest("GET", "/api/settings", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /api/settings returned status %d, expected %d", w.Code, http.StatusOK)
	}

	// Test POST /api/settings invalid json -> 400
	req, _ = http.NewRequest("POST", "/api/settings", bytes.NewBufferString("{invalid-json"))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /api/settings invalid json returned status %d, expected %d", w.Code, http.StatusBadRequest)
	}
}

func TestSecurityAuthMiddleware(t *testing.T) {
	router := setupRouter()

	initialCfg, _ := config.LoadConfig()
	defer func() {
		config.SaveConfig(initialCfg)
		os.Remove("config.yml")
	}()

	// Case 1: Disabled settings
	disabledCfg := initialCfg
	disabledCfg.Security.EnableSettings = false
	config.SaveConfig(disabledCfg)

	req, _ := http.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when settings disabled, got %d", w.Code)
	}

	// Case 2: Protected with access key
	keyCfg := initialCfg
	keyCfg.Security.EnableSettings = true
	keyCfg.Security.AccessKey = "secret123"
	keyCfg.Security.AdminPassword = ""
	config.SaveConfig(keyCfg)

	// Without key -> 401
	req, _ = http.NewRequest("GET", "/settings", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without access key, got %d", w.Code)
	}

	// With query param key -> 200
	req, _ = http.NewRequest("GET", "/settings?key=secret123", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid access key, got %d", w.Code)
	}

	// Case 3: Protected with admin password (Basic Auth)
	passCfg := initialCfg
	passCfg.Security.EnableSettings = true
	passCfg.Security.AccessKey = ""
	passCfg.Security.AdminPassword = "mypassword"
	config.SaveConfig(passCfg)

	// Without auth -> 401
	req, _ = http.NewRequest("GET", "/settings", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without password, got %d", w.Code)
	}

	// With valid basic auth -> 200
	req, _ = http.NewRequest("GET", "/settings", nil)
	req.SetBasicAuth("admin", "mypassword")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid basic auth, got %d", w.Code)
	}
}
