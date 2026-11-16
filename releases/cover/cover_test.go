package cover

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildCover(t *testing.T) {
	// Create dummy test image
	img := image.NewRGBA(image.Rect(0, 0, 100, 150))
	for y := 0; y < 150; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 50, B: 50, A: 255})
		}
	}
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		t.Fatalf("failed to encode dummy png: %v", err)
	}
	imgBytes := buf.Bytes()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(imgBytes)
	}))
	defer ts.Close()

	tmpDir, err := os.MkdirTemp("", "cover_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outPath := filepath.Join(tmpDir, "out_cover.png")

	// Test 1: Empty URLs -> should return error
	if err := BuildCover(nil, "", outPath); err == nil {
		t.Errorf("expected error for empty urls")
	}

	// Test 2: Build cover with 1 image
	if err := BuildCover([]string{ts.URL + "/poster1.png"}, ts.URL+"/backdrop.png", outPath); err != nil {
		t.Fatalf("BuildCover failed with 1 poster: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected output file to exist: %v", err)
	}

	// Test 3: Build cover with 4 images
	urls := []string{
		ts.URL + "/1.png",
		ts.URL + "/2.png",
		ts.URL + "/3.png",
		ts.URL + "/4.png",
	}
	if err := BuildCover(urls, "", outPath); err != nil {
		t.Fatalf("BuildCover failed with 4 posters: %v", err)
	}
}
