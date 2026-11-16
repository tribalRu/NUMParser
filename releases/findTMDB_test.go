package releases

import (
	"testing"
)

func TestExtractIDsFromHTML(t *testing.T) {
	htmlSample := `
<html>
<body>
<table id="details">
	<tr>
		<td>
			<a href="https://www.imdb.com/title/tt1375666/?ref_=fn_al_tt_1">IMDB</a>
			<a href="https://kinopoisk.ru/film/447301/#reviews">Кинопоиск</a>
		</td>
	</tr>
</table>
</body>
</html>
`
	imdbID, kpID := extractIDsFromHTML(htmlSample)
	if imdbID != "tt1375666" {
		t.Errorf("expected imdbID 'tt1375666', got %q", imdbID)
	}
	if kpID != "447301" {
		t.Errorf("expected kpID '447301', got %q", kpID)
	}
}

func TestExtractIDsFromHTMLEmpty(t *testing.T) {
	imdbID, kpID := extractIDsFromHTML("<html><body><div>No table</div></body></html>")
	if imdbID != "" || kpID != "" {
		t.Errorf("expected empty IDs, got imdb: %q, kp: %q", imdbID, kpID)
	}
}

