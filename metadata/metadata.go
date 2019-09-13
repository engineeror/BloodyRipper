package metadata

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type (
	Album struct {
		Album, Artist, Year, Genre string
		Tracks                     Tracks
	}

	Albums []Album

	Track struct {
		Num, Title, Artist, Comment string
		FileName                    string
	}

	Tracks []Track
)

const (
	httpAgentMB = "BloodyRipper/0.01a ( https://github.com/engineeror/BloodyRipper )"
	// httpAgentDC = "BloodyRipper/0.01a +https://github.com/engineeror/BloodyRipper" // discogs' recommended agent
)

func init() {
	//
}

// SetFormatting overrides the default formatting for folder and file names TODO
func SetFormatting() {

}

// QueryMusicBrainz fetches Album metadata from the MusicBrainz DB
func QueryMusicBrainz(discID string) (Albums, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("https://musicbrainz.org/ws/2/discid/%s?fmt=json&inc=artist-credits+recordings", discID), nil) // discID doesn't need escaping
	// req, err := http.NewRequest("GET", "https://www.whatismybrowser.com/detect/what-http-headers-is-my-browser-sending", nil) // discID doesn't need escaping
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", httpAgentMB)

	resp, err := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if err != nil {
		return nil, err
	}

	releases := &struct {
		Releases []*struct {
			// MBID             string `json:"id"`
			// Country, Barcode string

			Title, Date string
			Artists     []*struct{ Name, Joinphrase string } `json:"artist-credit"`
			Media       []*struct {
				Format string
				Tracks []*struct{ Number, Title string }
				Discs  []*struct{ Id string }
			}
		}
	}{}
	if err = json.NewDecoder(resp.Body).Decode(releases); err != nil {
		return nil, err
	}

	releaseCount := len(releases.Releases)
	if releaseCount == 0 {
		return nil, errors.New("disc not found in MusicBrainz database")
	}

	var albums Albums
	for _, r := range releases.Releases {
		var artist string
		switch count := len(r.Artists); {
		case count == 1:
			artist = r.Artists[0].Name
		case count > 1:
			var b strings.Builder
			for _, a := range r.Artists {
				b.WriteString(a.Name)
				b.WriteString(a.Joinphrase)
			}
			artist = b.String()
		}

		var tracks Tracks
		// r.Media contains all discs of a multi-disc album, even though we queried a particular discID
		for _, m := range r.Media {
			if m.Format != "CD" {
				continue
			}

			match := false
			for _, d := range m.Discs {
				if d.Id == discID {
					match = true // m is the media matching our queried discID
					break
				}
			}
			if !match {
				continue
			}

			tracks = make(Tracks, len(m.Tracks))
			for i, t := range m.Tracks {
				tracks[i].Num = t.Number
				tracks[i].Title = t.Title
				tracks[i].Artist = artist
			}

			break
		}

		a := Album{
			Album:  r.Title,
			Artist: artist,
			Year:   r.Date[:4], // sometimes it's already 4 runes comprising a year. I hope the formatting is consistent...
			Tracks: tracks,
		}

		if b, _ := albums.contains(&a); b {
			continue
		}
		albums = append(albums, a)
	}

	return albums, nil
}

// returns true if Albums contains an Album matching relevant fields of the supplied Album. returns index of matching Album
func (albums Albums) contains(album *Album) (bool, int) {
	for i, a := range albums {
		if a.Artist != album.Artist || a.Album != album.Album || a.Year != album.Year {
			continue
		}

		for i2, t := range a.Tracks {
			t2 := album.Tracks[i2]
			if t.Title != t2.Title || t.Artist != t2.Artist {
				goto continuation
			}

		}

		return true, i
	continuation:
	}

	return false, -1
}
