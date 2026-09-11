package decide

import (
	"reflect"
	"testing"

	"github.com/santiagosayshey/recall/internal/webhook"
)

func TestDecide(t *testing.T) {
	hdr := webhook.Header{App: webhook.Radarr, Instance: "Radarr", DownloadID: "abc", Media: webhook.Media{ID: 11, Title: "100% Wolf", Year: 2020}}
	grab := webhook.Grab{Header: hdr, ReleaseTitle: "grabbed", Score: 881400, Formats: []string{"1080p Bluray", "1080p Quality Tier 5", "Dolby Digital"}}
	imp := webhook.Import{Header: hdr, ReleaseTitle: "grabbed", FileName: "imported", Path: "/x.mkv", Score: -299599, Formats: []string{"1080p Bluray", "Dolby Digital", "Release Group (Missing)"}}

	base := Decision{
		App: webhook.Radarr, Instance: "Radarr", DownloadID: "abc", Media: hdr.Media,
		ReleaseTitle: "grabbed", FileName: "imported", Path: "/x.mkv",
		ImportScore: -299599, Lost: []string{}, Gained: []string{},
	}
	with := func(f func(*Decision)) Decision { d := base; f(&d); return d }

	tests := []struct {
		name string
		grab *webhook.Grab
		imp  webhook.Import
		want Decision
	}{
		{"drift", &grab, imp, with(func(d *Decision) {
			d.Result, d.GrabScore, d.Delta = Drift, 881400, -1180999
			d.Lost, d.Gained = []string{"1080p Quality Tier 5"}, []string{"Release Group (Missing)"}
		})},
		{"clean when equal", &grab, func() webhook.Import { i := imp; i.Score, i.Formats = grab.Score, grab.Formats; return i }(), with(func(d *Decision) {
			d.Result, d.GrabScore, d.ImportScore = Clean, 881400, 881400
		})},
		{"clean when higher", &grab, func() webhook.Import { i := imp; i.Score = 900000; return i }(), with(func(d *Decision) {
			d.Result, d.GrabScore, d.ImportScore, d.Delta = Clean, 881400, 900000, 18600
			d.Lost, d.Gained = []string{"1080p Quality Tier 5"}, []string{"Release Group (Missing)"}
		})},
		{"unmatched", nil, imp, with(func(d *Decision) { d.Result = Unmatched })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Decide(tt.grab, tt.imp); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestMissingKeepsOrderAndNeverNil(t *testing.T) {
	if got := missing([]string{"c", "a", "b"}, []string{"a"}); !reflect.DeepEqual(got, []string{"c", "b"}) {
		t.Errorf("got %v", got)
	}
	if got := missing(nil, nil); got == nil || len(got) != 0 {
		t.Errorf("got %#v, want empty non-nil", got)
	}
}
