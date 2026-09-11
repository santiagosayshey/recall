// Package decide compares an import with its grab. It is pure: two records
// in, one decision out, nothing touched.
package decide

import "github.com/santiagosayshey/recall/internal/webhook"

// Result is what the comparison found.
type Result string

const (
	Clean     Result = "clean"     // the import scored at least what the grab did
	Drift     Result = "drift"     // the import scored lower than the grab
	Unmatched Result = "unmatched" // no grab to compare against
)

// Decision is one import weighed against its grab. It carries what an
// alert needs to read on its own. GrabScore and Delta are zero when
// Unmatched.
type Decision struct {
	App          webhook.App   `json:"app"`
	Instance     string        `json:"instance"`
	DownloadID   string        `json:"downloadId"`
	Media        webhook.Media `json:"media"`
	Result       Result        `json:"result"`
	ReleaseTitle string        `json:"releaseTitle"` // what was grabbed
	FileName     string        `json:"fileName"`     // what was scored at import
	Path         string        `json:"path"`
	GrabScore    int           `json:"grabScore"`
	ImportScore  int           `json:"importScore"`
	Delta        int           `json:"delta"`  // import minus grab
	Lost         []string      `json:"lost"`   // formats the grab had and the import does not
	Gained       []string      `json:"gained"` // formats the import has and the grab did not
}

// Decide weighs an import against its grab. A nil grab is Unmatched.
func Decide(g *webhook.Grab, i webhook.Import) Decision {
	d := Decision{
		App:          i.App,
		Instance:     i.Instance,
		DownloadID:   i.DownloadID,
		Media:        i.Media,
		Result:       Unmatched,
		ReleaseTitle: i.ReleaseTitle,
		FileName:     i.FileName,
		Path:         i.Path,
		ImportScore:  i.Score,
		Lost:         []string{},
		Gained:       []string{},
	}
	if g == nil {
		return d
	}
	d.GrabScore = g.Score
	d.Delta = i.Score - g.Score
	d.Lost = missing(g.Formats, i.Formats)
	d.Gained = missing(i.Formats, g.Formats)
	d.Result = Clean
	if i.Score < g.Score {
		d.Result = Drift
	}
	return d
}

// missing returns the names in from that are not in against, in from's
// order. Never nil, so the JSON is [] rather than null.
func missing(from, against []string) []string {
	have := make(map[string]bool, len(against))
	for _, s := range against {
		have[s] = true
	}
	out := []string{}
	for _, s := range from {
		if !have[s] {
			out = append(out, s)
		}
	}
	return out
}
