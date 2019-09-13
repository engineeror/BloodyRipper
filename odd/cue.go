package odd

import "main/metadata"

// Cue returns the CUE sheet formatted similarly as EAC's
// separate specifies whether the ripped tracks are saved as separate files
func (drive *Drive) Cue(separate bool, album metadata.Album) string {
	mut.Lock()
	defer mut.Unlock()
	if !drive.hasMedia /*disc removed*/ || drive.d == nil /*drive obj got Destroy()ed*/ {
		return ""
	}

	if drive.numOfTracks() != (uint8)(len(album.Tracks)) {
		panic("can generate CUEs for complete discs only")
	}

	return ""
}
