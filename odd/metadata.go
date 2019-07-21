package odd

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"strings"
)

type Album struct {
	Album, Year string
	Catalog     string // Contains the UPC/EAN code of the disc
	Tracks      []*TrackMetadata
}

// TrackMetadata holds all the metadata a track can have
type TrackMetadata struct {
	Num                                    uint8
	Name, Artist, Album, Comment, Composer string
	FileName                               string
}

var ()
// strings.Title

// SetFormatting overrides the default formatting for folder and file names TODO
func SetFormatting() {

}

// Acquire fetches Album metadata from remote provider TODO
func Acquire() {
	// BloodyRipper/0.01a ( https://github.com/engineeror/BloodyRipper )
}

// CalcDiscID calculates the musicbrainz Disc ID https://wiki.musicbrainz.org/Disc_ID_Calculation#Calculating_the_Disc_ID
func (drive *Drive) CalcDiscID() string {
	m.Lock()
	defer m.Unlock()
	if !drive.hasMedia /*disc removed*/ || drive.d == nil /*drive obj got Destroy()ed*/ {
		return ""
	}

	num := uint8(len(drive.Tracks))
	h := sha1.New()

	_, err := fmt.Fprintf(h, "%02X", drive.Tracks[0].Num)
	if err != nil {
		return ""
	}

	_, err = fmt.Fprintf(h, "%02X", drive.Tracks[num-1].Num)
	if err != nil {
		return ""
	}

	_, err = fmt.Fprintf(h, "%08X", drive.leadOutTrackOffset())
	if err != nil {
		return ""
	}

	n := uint8(0)
	for ; n < num; n++ {
		_, err = fmt.Fprintf(h, "%08X", drive.Tracks[n].offset())
		if err != nil {
			return ""
		}
	}
	for ; n < 99; n++ {
		_, err = fmt.Fprintf(h, "%08X", 0)
		if err != nil {
			return ""
		}
	}

	h.BlockSize()
	sum := h.Sum(nil)
	b := strings.Builder{}

	encoder := base64.NewEncoder(base64.StdEncoding, &b)
	_, err = encoder.Write(sum)
	if err != nil {
		return ""
	}

	err = encoder.Close()
	if err != nil {
		return ""
	}

	r := strings.NewReplacer("+", ".", "/", "_", "=", "-") // I don't think go lib can produce the musicbrainz format directly

	return r.Replace(b.String())
}
