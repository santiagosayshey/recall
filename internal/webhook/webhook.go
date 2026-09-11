// Package webhook turns a raw Radarr or Sonarr webhook body into one event.
// The two apps shape their bodies differently, and Sonarr sends two kinds of
// import under one name; nothing after this package needs to know that.
package webhook

import (
	"encoding/json"
	"errors"
	"path"
)

// Kind is what an event is to Recall. Most webhooks are Ignored.
type Kind string

const (
	Grab    Kind = "grab"
	Import  Kind = "import"
	Ignored Kind = "ignored"
)

// App is which program sent the event.
type App string

const (
	Radarr App = "radarr"
	Sonarr App = "sonarr"
)

// ErrNotWebhook is returned for a body with no event type at all.
var ErrNotWebhook = errors.New("not an *arr webhook")

// Event is the normalised form of a webhook. Fields after DownloadID are set
// for grabs and imports only.
type Event struct {
	Kind       Kind
	App        App
	Instance   string
	EventType  string // the app's own name for the event
	DownloadID string

	Media        Media
	ReleaseTitle string   // the indexer's title, on both grab and import
	FileName     string   // import only: the name the file was scored as, when the app says
	Path         string   // import only: where the file landed
	Group        string   // release group, when parsed
	Quality      string   // for example Bluray-1080p
	Score        int      // custom format score for this event
	Formats      []string // custom format names, in the app's order
	IsUpgrade    bool     // import only

	Raw json.RawMessage // the body as received
}

// Media is the movie or the episodes the event is about, flattened so both
// apps look alike.
type Media struct {
	ID       int // the app's own id for the movie or series
	Title    string
	Year     int
	Episodes []Episode // Sonarr only
}

type Episode struct {
	Season int
	Number int
}

// body is the union of what Radarr and Sonarr send. Pointers tell presence
// apart from emptiness, which is how the shapes are told apart.
type body struct {
	EventType    string `json:"eventType"`
	InstanceName string `json:"instanceName"`
	DownloadID   string `json:"downloadId"`
	IsUpgrade    bool   `json:"isUpgrade"`
	Movie        *struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
		Year  int    `json:"year"`
	} `json:"movie"`
	Series *struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
		Year  int    `json:"year"`
	} `json:"series"`
	Episodes []struct {
		Season int `json:"seasonNumber"`
		Number int `json:"episodeNumber"`
	} `json:"episodes"`
	Release *struct {
		Title   string `json:"releaseTitle"`
		Group   string `json:"releaseGroup"`
		Quality string `json:"quality"`
	} `json:"release"`
	MovieFile    *file `json:"movieFile"`
	EpisodeFile  *file `json:"episodeFile"`
	EpisodeFiles []any `json:"episodeFiles"`
	Formats      *struct {
		Score   int `json:"customFormatScore"`
		Formats []struct {
			Name string `json:"name"`
		} `json:"customFormats"`
	} `json:"customFormatInfo"`
}

type file struct {
	Path      string `json:"path"`
	Relative  string `json:"relativePath"`
	SceneName string `json:"sceneName"`
	Group     string `json:"releaseGroup"`
	Quality   string `json:"quality"`
}

// Parse reads one body. A body with no event type is an error; every other
// body parses, and the ones Recall does not act on come back Ignored.
func Parse(raw []byte) (Event, error) {
	var b body
	if err := json.Unmarshal(raw, &b); err != nil || b.EventType == "" {
		return Event{}, ErrNotWebhook
	}
	e := Event{
		Kind:       Ignored,
		Instance:   b.InstanceName,
		EventType:  b.EventType,
		DownloadID: b.DownloadID,
		Raw:        append(json.RawMessage(nil), raw...),
	}
	switch {
	case b.Movie != nil:
		e.App = Radarr
		e.Media = Media{ID: b.Movie.ID, Title: b.Movie.Title, Year: b.Movie.Year}
	case b.Series != nil:
		e.App = Sonarr
		e.Media = Media{ID: b.Series.ID, Title: b.Series.Title, Year: b.Series.Year}
		for _, ep := range b.Episodes {
			e.Media.Episodes = append(e.Media.Episodes, Episode{Season: ep.Season, Number: ep.Number})
		}
	}
	if b.Formats == nil || b.Release == nil {
		return e, nil // nothing to score: Test, Health, MovieAdded, Sonarr's import summary
	}
	e.ReleaseTitle = b.Release.Title
	e.Score = b.Formats.Score
	for _, f := range b.Formats.Formats {
		e.Formats = append(e.Formats, f.Name)
	}
	switch {
	case b.EventType == "Grab":
		e.Kind = Grab
		e.Group = b.Release.Group
		e.Quality = b.Release.Quality
	case b.EventType == "Download" && (b.MovieFile != nil || b.EpisodeFile != nil):
		e.Kind = Import
		f := b.MovieFile
		if f == nil {
			f = b.EpisodeFile
		}
		e.FileName = f.SceneName
		if e.FileName == "" {
			e.FileName = path.Base(f.Relative)
		}
		e.Path = f.Path
		e.Group = f.Group
		e.Quality = f.Quality
		e.IsUpgrade = b.IsUpgrade
	default:
		e.ReleaseTitle, e.Score, e.Formats = "", 0, nil
	}
	return e, nil
}
