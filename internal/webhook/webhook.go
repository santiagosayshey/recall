// Package webhook turns a raw Radarr or Sonarr webhook body into one event.
// The two apps shape their bodies differently, and Sonarr sends two kinds of
// import under one name; nothing after this package needs to know that.
package webhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
)

// App is which program sent the event.
type App string

const (
	Radarr App = "radarr"
	Sonarr App = "sonarr"
)

// ErrNotWebhook is returned for a body with no event type at all.
var ErrNotWebhook = errors.New("not an *arr webhook")

// Event is one parsed body: a *Grab, an *Import, or an *Other for a body
// Recall does not act on, such as Test or Health. Switch on the type.
type Event interface {
	header() Header
}

// Other is any body that is neither a grab nor an import.
type Other struct {
	Header
}

// Header is what every body carries. The JSON names are the ones the store
// writes; Raw is left to the store so it can go last on the line.
type Header struct {
	App        App             `json:"app"`
	Instance   string          `json:"instance"`
	EventType  string          `json:"eventType"` // the app's own name for the event
	DownloadID string          `json:"downloadId"`
	Media      Media           `json:"media"`
	Raw        json.RawMessage `json:"-"` // the body as received
}

// Grab is a release the app chose, scored against the indexer's title.
type Grab struct {
	Header
	ReleaseTitle string   `json:"releaseTitle"`
	Group        string   `json:"group,omitempty"` // release group, when parsed
	Quality      string   `json:"quality"`         // for example Bluray-1080p
	Score        int      `json:"score"`
	Formats      []string `json:"formats"` // custom format names, in the app's order
}

// Import is a file the app took into the library, scored against the file.
type Import struct {
	Header
	ReleaseTitle string   `json:"releaseTitle"`    // the indexer's title, again
	FileName     string   `json:"fileName"`        // the name the file was scored as, when the app says
	Path         string   `json:"path"`            // where the file landed
	Group        string   `json:"group,omitempty"` // release group, when parsed
	Quality      string   `json:"quality"`         // for example Bluray-1080p
	Score        int      `json:"score"`
	Formats      []string `json:"formats"` // custom format names, in the app's order
	IsUpgrade    bool     `json:"isUpgrade"`
}

// Media is the movie or the episodes the event is about, flattened so both
// apps look alike.
type Media struct {
	ID       int       `json:"id"` // the app's own id for the movie or series
	Title    string    `json:"title"`
	Year     int       `json:"year"`
	Episodes []Episode `json:"episodes,omitempty"` // Sonarr only
}

type Episode struct {
	Season int `json:"season"`
	Number int `json:"number"`
}

func (h Header) header() Header { return h }

// String is the media as a person would name it: "100% Wolf (2020)" or
// "Sherlock (2010) S01E02", with every episode listed for a pack.
func (m Media) String() string {
	s := fmt.Sprintf("%s (%d)", m.Title, m.Year)
	for i, ep := range m.Episodes {
		if i == 0 {
			s += " "
		} else {
			s += ","
		}
		s += fmt.Sprintf("S%02dE%02d", ep.Season, ep.Number)
	}
	return s
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
	MovieFile   *file `json:"movieFile"`
	EpisodeFile *file `json:"episodeFile"`
	Formats     *struct {
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
// body parses, and the ones Recall does not act on come back as Other.
func Parse(raw []byte) (Event, error) {
	var b body
	if err := json.Unmarshal(raw, &b); err != nil || b.EventType == "" {
		return nil, ErrNotWebhook
	}
	h := Header{
		Instance:   b.InstanceName,
		EventType:  b.EventType,
		DownloadID: b.DownloadID,
		Raw:        append(json.RawMessage(nil), raw...),
	}
	switch {
	case b.Movie != nil:
		h.App = Radarr
		h.Media = Media{ID: b.Movie.ID, Title: b.Movie.Title, Year: b.Movie.Year}
	case b.Series != nil:
		h.App = Sonarr
		h.Media = Media{ID: b.Series.ID, Title: b.Series.Title, Year: b.Series.Year}
		for _, ep := range b.Episodes {
			h.Media.Episodes = append(h.Media.Episodes, Episode{Season: ep.Season, Number: ep.Number})
		}
	}
	if b.Formats == nil || b.Release == nil {
		return &Other{h}, nil // nothing to score: Test, Health, MovieAdded, Sonarr's import summary
	}
	var formats []string
	for _, f := range b.Formats.Formats {
		formats = append(formats, f.Name)
	}
	switch {
	case b.EventType == "Grab":
		return &Grab{
			Header:       h,
			ReleaseTitle: b.Release.Title,
			Group:        b.Release.Group,
			Quality:      b.Release.Quality,
			Score:        b.Formats.Score,
			Formats:      formats,
		}, nil
	case b.EventType == "Download" && (b.MovieFile != nil || b.EpisodeFile != nil):
		f := b.MovieFile
		if f == nil {
			f = b.EpisodeFile
		}
		name := f.SceneName
		if name == "" {
			name = path.Base(f.Relative)
		}
		return &Import{
			Header:       h,
			ReleaseTitle: b.Release.Title,
			FileName:     name,
			Path:         f.Path,
			Group:        f.Group,
			Quality:      f.Quality,
			Score:        b.Formats.Score,
			Formats:      formats,
			IsUpgrade:    b.IsUpgrade,
		}, nil
	}
	return &Other{h}, nil // scored, but not a kind Recall handles
}
