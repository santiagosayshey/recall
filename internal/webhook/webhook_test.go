package webhook

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func load(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestParse(t *testing.T) {
	wolf := Media{ID: 11, Title: "100% Wolf", Year: 2020}
	sherlock := func(eps ...Episode) Media {
		return Media{ID: 6, Title: "Sherlock", Year: 2010, Episodes: eps}
	}
	s1 := []Episode{{1, 1}, {1, 2}, {1, 3}}
	honeFormats := []string{"1080p WEB-DL", "1080p WEB-DL HEVC Tier 1", "5.1", "Amazon Enhancement", "AMZN", "Dolby Digital +", "h265", "Season Pack", "WEB Tier 01", "WEB-DL Tier 2", "x265 (HD)", "x265 (no HDR/DV)"}
	playFormats := []string{"1080p WEB-DL", "1080p WEB-DL (Efficient)", "1080p WEB-DL (h264)", "5.1", "Anime Web Tier 05", "Anime WEB Tier 05", "Dolby Digital", "NF", "WEB Tier 02", "WEB-DL Tier 2"}
	honeTitle := "Sherlock (2010) S01 (1080p AMZN WEB-DL H265 SDR DDP 5 1 English - HONE)"
	playTitle := "Sherlock S02E01 A Scandal in Belgravia 1080p NF WEB-DL DD 5 1 H 264-playWEB"

	tests := []struct {
		name string
		want Event
	}{
		{"radarr-grab", Event{
			Kind: Grab, App: Radarr, Instance: "Radarr", EventType: "Grab",
			DownloadID:   "0000000000000000000000000000000000000001",
			Media:        wolf,
			ReleaseTitle: "100 Percent Wolf 2020 1080p BluRay DD5.1 x264-PTer",
			Group:        "PTer", Quality: "Bluray-1080p", Score: 881400,
			Formats: []string{"1080p Bluray", "1080p Quality Tier 5", "Dolby Digital"},
		}},
		{"radarr-import", Event{
			Kind: Import, App: Radarr, Instance: "Radarr", EventType: "Download",
			DownloadID:   "0000000000000000000000000000000000000001",
			Media:        wolf,
			ReleaseTitle: "100 Percent Wolf 2020 1080p BluRay DD5.1 x264-PTer",
			FileName:     "100 Percent Wolf.2020.1080p.BluRay.DD5.1.x264- PTer",
			Path:         "/media/test-library/100% Wolf (2020) {tmdb-520946}/100% Wolf (2020) {tmdb-520946} [Bluray-1080p][AC3 5.1][x264].mkv",
			Quality:      "Bluray-1080p", Score: -299599,
			Formats: []string{"1080p Bluray", "Dolby Digital", "Release Group (Missing)"},
		}},
		{"sonarr-grab-episode", Event{
			Kind: Grab, App: Sonarr, Instance: "Sonarr", EventType: "Grab",
			DownloadID:   "0000000000000000000000000000000000000003",
			Media:        sherlock(Episode{2, 1}),
			ReleaseTitle: playTitle,
			Group:        "playWEB", Quality: "WEBDL-1080p", Score: 861480,
			Formats: playFormats,
		}},
		{"sonarr-grab-pack", Event{
			Kind: Grab, App: Sonarr, Instance: "Sonarr", EventType: "Grab",
			DownloadID:   "0000000000000000000000000000000000000004",
			Media:        sherlock(s1...),
			ReleaseTitle: honeTitle,
			Group:        "HONE", Quality: "WEBDL-1080p", Score: 923690,
			Formats: honeFormats,
		}},
		{"sonarr-grab-pack-no-group", Event{
			Kind: Grab, App: Sonarr, Instance: "Sonarr", EventType: "Grab",
			DownloadID:   "0000000000000000000000000000000000000002",
			Media:        sherlock(s1...),
			ReleaseTitle: "Sherlock S01 Complete 720p BRRip x264 AAC - M@X",
			Quality:      "Bluray-720p", Score: 540210,
			Formats: []string{"720p Bluray", "AAC", "Group Missing", "Release Group (Missing)", "Season Pack"},
		}},
		{"sonarr-import-episode", Event{
			Kind: Import, App: Sonarr, Instance: "Sonarr", EventType: "Download",
			DownloadID:   "0000000000000000000000000000000000000003",
			Media:        sherlock(Episode{2, 1}),
			ReleaseTitle: playTitle,
			FileName:     playTitle,
			Path:         "/media/test-library/Sherlock (2010) {tvdb-176941}/Season 02/Sherlock (2010) - S02E01 - A Scandal in Belgravia [NF][WEBDL-1080p][EAC3 5.1][x264]-playWEB.mkv",
			Group:        "playWEB", Quality: "WEBDL-1080p", Score: 861480,
			Formats: playFormats,
		}},
		{"sonarr-import-pack-e02", Event{
			Kind: Import, App: Sonarr, Instance: "Sonarr", EventType: "Download",
			DownloadID:   "0000000000000000000000000000000000000004",
			Media:        sherlock(Episode{1, 2}),
			ReleaseTitle: honeTitle,
			// A pack's files carry no scene name, so the file name stands in.
			FileName: "Sherlock (2010) - S01E02 - The Blind Banker [AMZN][WEBDL-1080p][EAC3 5.1][h265]-HONE.mkv",
			Path:     "/media/test-library/Sherlock (2010) {tvdb-176941}/Season 01/Sherlock (2010) - S01E02 - The Blind Banker [AMZN][WEBDL-1080p][EAC3 5.1][h265]-HONE.mkv",
			Group:    "HONE", Quality: "WEBDL-1080p", Score: 923690,
			Formats: honeFormats,
		}},
		// Everything else is ignored, with the app and media still filled in
		// where the body has them.
		{"radarr-test", Event{Kind: Ignored, App: Radarr, Instance: "Radarr", EventType: "Test", Media: Media{ID: 1, Title: "Test Title", Year: 1970}}},
		{"radarr-movie-added", Event{Kind: Ignored, App: Radarr, Instance: "Radarr", EventType: "MovieAdded", Media: wolf}},
		{"sonarr-test", Event{Kind: Ignored, App: Sonarr, Instance: "Sonarr", EventType: "Test", Media: Media{ID: 1, Title: "Test Title", Episodes: []Episode{{1, 1}}}}},
		{"sonarr-series-add", Event{Kind: Ignored, App: Sonarr, Instance: "Sonarr", EventType: "SeriesAdd", Media: sherlock()}},
		{"sonarr-health", Event{Kind: Ignored, Instance: "Sonarr", EventType: "Health"}},
		{"sonarr-import-episode-summary", Event{Kind: Ignored, App: Sonarr, Instance: "Sonarr", EventType: "Download", DownloadID: "0000000000000000000000000000000000000003", Media: sherlock(Episode{2, 1})}},
		{"sonarr-import-pack-summary", Event{Kind: Ignored, App: Sonarr, Instance: "Sonarr", EventType: "Download", DownloadID: "0000000000000000000000000000000000000004", Media: sherlock(s1...)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := load(t, tt.name)
			got, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Raw, raw) {
				t.Error("raw body not kept")
			}
			got.Raw = nil
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestParsePackImportsShareTheGrab(t *testing.T) {
	grab, _ := Parse(load(t, "sonarr-grab-pack"))
	for _, name := range []string{"sonarr-import-pack-e01", "sonarr-import-pack-e02", "sonarr-import-pack-e03"} {
		imp, _ := Parse(load(t, name))
		if imp.Kind != Import || imp.DownloadID != grab.DownloadID {
			t.Errorf("%s: kind %s, download %s; want an import of %s", name, imp.Kind, imp.DownloadID, grab.DownloadID)
		}
		if len(imp.Media.Episodes) != 1 {
			t.Errorf("%s: %d episodes, want one per import", name, len(imp.Media.Episodes))
		}
	}
}

func TestParseRejectsNonWebhooks(t *testing.T) {
	for _, raw := range []string{``, `{}`, `[]`, `hello`, `{"instanceName":"Radarr"}`} {
		if _, err := Parse([]byte(raw)); err != ErrNotWebhook {
			t.Errorf("%q: err %v, want ErrNotWebhook", raw, err)
		}
	}
}
