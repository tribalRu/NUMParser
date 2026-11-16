package parser

import (
	"NUMParser/db/models"
	"encoding/json"
	"testing"
)

func TestParseTitle_Classification(t *testing.T) {
	p := NewRutor()

	// 1. Cartoon Movie with brackets
	torrMovie := &models.TorrentDetails{
		Title: "Шрек / Shrek (2001) BDRip [1080p]",
	}
	p.parseTitle(torrMovie, "7")
	if torrMovie.Categories != models.CatCartoonMovie {
		t.Errorf("expected CatCartoonMovie for Shrek, got %s", torrMovie.Categories)
	}
	if torrMovie.Year != 2001 {
		t.Errorf("expected year 2001, got %d", torrMovie.Year)
	}

	// 2. Cartoon Series with season marker
	torrSeries := &models.TorrentDetails{
		Title: "Симпсоны / The Simpsons (1989-2023) WEB-DL [Сезон 1-34] [01-22 из 22]",
	}
	p.parseTitle(torrSeries, "7")
	if torrSeries.Categories != models.CatCartoonSeries {
		t.Errorf("expected CatCartoonSeries for Simpsons, got %s", torrSeries.Categories)
	}

	// 3. Regular Movie
	torrRegular := &models.TorrentDetails{
		Title: "Веном / Venom (2018) BDRip",
	}
	p.parseTitle(torrRegular, "1")
	if torrRegular.Categories != models.CatMovie {
		t.Errorf("expected CatMovie, got %s", torrRegular.Categories)
	}
}

func TestGetHash(t *testing.T) {
	magnetLower := "magnet:?xt=urn:btih:3b8c35bb5f9175396a40a586ec32b95b8782a174&dn=rutor.info"
	if h := getHash(magnetLower); h != "3b8c35bb5f9175396a40a586ec32b95b8782a174" {
		t.Errorf("expected lowercase hash, got %q", h)
	}

	magnetUpper := "magnet:?xt=urn:BTIH:3B8C35BB5F9175396A40A586EC32B95B8782A174&dn=rutor.info"
	if h := getHash(magnetUpper); h != "3b8c35bb5f9175396a40a586ec32b95b8782a174" {
		t.Errorf("expected normalized lowercase hash for uppercase input, got %q", h)
	}

	magnetNoAmp := "magnet:?xt=urn:btih:3b8c35bb5f9175396a40a586ec32b95b8782a174"
	if h := getHash(magnetNoAmp); h != "3b8c35bb5f9175396a40a586ec32b95b8782a174" {
		t.Errorf("expected hash without amp, got %q", h)
	}

	if h := getHash("invalid_magnet"); h != "" {
		t.Errorf("expected empty string for invalid magnet, got %q", h)
	}
}

func TestParseQualities(t *testing.T) {
	// Video
	if q := ParseVQuality("BDRip 1080p"); q != models.Q_BDRIP_1080 {
		t.Errorf("expected Q_BDRIP_1080, got %d", q)
	}
	if q := ParseVQuality("UHD BDRemux 2160p Dolby Vision HDR"); q != models.Q_UHD_BDREMUX_DV {
		t.Errorf("expected Q_UHD_BDREMUX_DV, got %d", q)
	}

	// Audio
	if a := ParseAQuality("Дублированный | Лицензия"); a != models.Q_LICENSE {
		t.Errorf("expected Q_LICENSE, got %d", a)
	}
	if a := ParseAQuality("D | iTunes"); a != models.Q_LICENSE {
		t.Errorf("expected Q_LICENSE, got %d", a)
	}
	if a := ParseAQuality("LostFilm"); a != models.Q_PS {
		t.Errorf("expected Q_PS for LostFilm, got %d", a)
	}
	if a := ParseAQuality("AniLibria"); a != models.Q_LS {
		t.Errorf("expected Q_LS for AniLibria, got %d", a)
	}
}

func TestParseDate_Safe(t *testing.T) {
	p := NewRutor()

	cases := []string{
		"16 Ноя 24",
		"16 Ноя 2024",
		"16\u00a0Ноя\u00a024",
		"1 Янв 23",
		"",
		"некорректная дата",
		"0",
	}

	for _, c := range cases {
		d := p.parseDate(c)
		if d.Year() < 1970 || d.Year() > 2100 {
			t.Errorf("parseDate(%q) returned out-of-range year %d", c, d.Year())
		}
		// Verify JSON marshalling succeeds
		torr := &models.TorrentDetails{
			Hash:       "abc",
			CreateDate: d,
		}
		if _, err := json.Marshal(torr); err != nil {
			t.Errorf("json.Marshal failed for torrent with date %v: %v", d, err)
		}
	}
}
