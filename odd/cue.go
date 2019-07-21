package odd

// Cue returns the CUE sheet formatted similarly as EAC's
// separate specifies whether the ripped tracks are joined into a single file or separate ones
func (drive *Drive) Cue(separate bool, tracks Tracks/*, album Album*/) string {
	m.Lock()
	defer m.Unlock()
	if !drive.hasMedia /*disc removed*/ || drive.d == nil /*drive obj got Destroy()ed*/ {
		return ""
	}

	if drive.numOfTracks() != (uint8)(len(tracks)) {
		panic("can generate CUEs for complete discs only")
	}

	return ""
}
