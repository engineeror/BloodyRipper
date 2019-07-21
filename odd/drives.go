package odd

// TODO: rewrite everything, get rid of this ugly & dangerous mutex rigmarole

/*
#cgo LDFLAGS: -lcdio -lcdio_cdda -lcdio_paranoia
#cgo pkg-config: gobject-2.0 gio-2.0
#include <stdint.h>
#include <stdbool.h>
#include <cdio/types.h>
#include <cdio/cdio.h>
#include <cdio/cd_types.h>
#include <cdio/paranoia/cdda.h>
#include <cdio/mmc_ll_cmds.h>
#include <cdio/mmc_hl_cmds.h>
#include <gio/gio.h>
#include <gobject/gsignal.h>

uint8_t countDrives(char *drives[]);
void VolumeRemovedCB(GVolumeMonitor *vm, GVolume *v, gpointer p);
void VolumeAddedCB(GVolumeMonitor *vm, GVolume *v, gpointer p);
*/
import "C"

import (
	"sync"
	"time"
	"unsafe"
)

type (
	Track struct {
		Num        uint8
		Duration   time.Duration
		Emphasis   bool
		CopyPermit bool

		drive *Drive
	}

	Tracks []*Track

	Drive struct {
		Path   string
		Name   string
		Tracks Tracks

		d        *C.cdrom_drive_t
		hasMedia bool // whether an Audio CD is currently present in drive. yes, there's cdrom_drive_t.opened...
	}

	// Drives is a simple map of Drive struct pointers, keyed with drive paths
	Drives map[string]*Drive
)

const (
	twoSecs = 150
)

var (
	// prevents g_main_loop racing against caller
	m sync.Mutex
	// Media change callbacks
	cb func(string, bool)

	// contains all drives returned with the last Get()
	availableDrives Drives
)

//export VolumeRemovedCB
func VolumeRemovedCB(_ *C.GVolumeMonitor, v *C.GVolume, _ C.gpointer) {
	m.Lock()
	defer m.Unlock()

	path := gvolume2Path(v)

	if drive, found := availableDrives[path]; found && cb != nil {
		drive.hasMedia = false
		cb(path, false)
	}
}

//export VolumeAddedCB
func VolumeAddedCB(_ *C.GVolumeMonitor, v *C.GVolume, _ C.gpointer) {
	m.Lock()
	defer m.Unlock()

	path := gvolume2Path(v)

	if drive, found := availableDrives[path]; found {
		if drive.initMedia() && /*must be last*/ cb != nil {
			cb(path, true)
		}
	}
}

// TODO: this is not nice, is it? Create an Init() func, to be called early in main()
func init() {
	availableDrives = make(Drives)

	loop := C.g_main_loop_new(nil, C.FALSE)
	// defer C.g_main_loop_unref(loop)

	volmon := C.g_volume_monitor_get()
	// defer C.g_object_unref(C.gpointer(volmon))

	C.g_signal_connect_data(C.gpointer(volmon), C.CString("volume-added"), C.GCallback(C.VolumeAddedCB), nil, nil, (C.GConnectFlags)(0))
	// defer C.g_signal_handler_disconnect(C.gpointer(volmon), handler)

	C.g_signal_connect_data(C.gpointer(volmon), C.CString("volume-removed"), C.GCallback(C.VolumeRemovedCB), nil, nil, (C.GConnectFlags)(0))
	// defer C.g_signal_handler_disconnect(C.gpointer(volmon), handler)

	// uncommenting any of those defers will break the signalling. kernel will release the resources on app exit
	// likewise this goroutine will be terminated ungracefully on app exit :/
	go func() {
		C.g_main_loop_run(loop)
		panic("☠️☠️☠️ g_main_loop_run() returned ☠️☠️☠️")
	}()
}

// Get returns all optical drives available on the system as a map keyed with their unix paths
// mediaChange() is called when an audio CD is either removed or inserted.
// hasMedia == true indicates hasMedia changed into valid Audio CD
// mediaChange() must not call any methods on any Drive object directly
func Get(mediaChange func(path string, media bool)) Drives {
	m.Lock()
	defer m.Unlock()

	if len(availableDrives) != 0 {
		panic("Every Get() call must be preceded with Destroy()ction for all Get()ed Drives")
	}

	devicesCArray := C.cdio_get_devices(C.DRIVER_DEVICE)
	if devicesCArray == nil || *devicesCArray == nil {
		return nil
	}
	defer C.cdio_free_device_list(devicesCArray)

	devices := deviceList2Slice(devicesCArray)

	var wg sync.WaitGroup
	drives := Drives{}
	for _, d := range devices {
		tmp := C.cdio_cddap_identify(d, 0, nil)
		if tmp == nil {
			continue
		}

		path := C.GoString(tmp.cdda_device_name)
		drives[path] = &Drive{d: tmp, Path: path, Name: C.GoString(tmp.drive_model)}
		availableDrives[path] = drives[path]
		cb = mediaChange

		// parallelize initMedia() 4 fun
		wg.Add(1)
		go func() {
			defer wg.Done()

			availableDrives[path].initMedia()
		}()
	}

	if len(drives) == 0 {
		return nil
	}

	wg.Wait() // wait for every initMedia() call to return

	return drives
}

func deviceList2Slice(cArray **C.char) []*C.char {
	deviceCount := uint8(C.countDrives(cArray))
	return (*[0xff]*C.char)(unsafe.Pointer(cArray))[:deviceCount:deviceCount /*capacity*/]
}

func gvolume2Path(volume *C.GVolume) string {
	return C.GoString(C.g_volume_get_identifier(volume, C.CString(C.G_DRIVE_IDENTIFIER_KIND_UNIX_DEVICE))) // deprecated, whatever
}

// this func is also used to verify whether the inserted disc is CDDA
func (drive *Drive) initMedia() bool {
	drive.hasMedia = C.cdio_cddap_open(drive.d) == 0

	drive.Tracks = drive.tracks()

	return drive.hasMedia
}

func (drive *Drive) firstLastIndices() (firstIndex, lastIndex uint8) {
	totalTracks := drive.numOfTracks()
	if totalTracks == 0 {
		return 0, 0
	}

	first := uint8(C.cdio_get_first_track_num(drive.d.p_cdio))
	firstIndex = 1 - first
	lastIndex = totalTracks - first

	return
}

// returns a slice of Track structs
func (drive *Drive) tracks() Tracks {
	msfDuration := func(i uint8, msf *C.msf_t) time.Duration { // TODO: isn't there a helper func like this in the C libs already, sector.h?
		deBCD := func(b C.uchar) time.Duration {
			return time.Duration(((b&0xF0)>>4)*10 + (b & 0x0F))
		}

		if !C.cdio_get_track_msf(drive.d.p_cdio, drive.d.disc_toc[i].bTrack, msf) {
			return 0
		}

		minutes := deBCD(msf.m) // readability ftw
		seconds := deBCD(msf.s)
		frames := deBCD(msf.f)

		return time.Minute*minutes + time.Second*seconds + time.Second/75*frames
	}

	firstIndex, lastIndex := drive.firstLastIndices()
	if lastIndex == 0 {
		return nil
	}

	var tracks Tracks
	msf := new(C.msf_t)
	startOfCurrent := msfDuration(firstIndex, msf)
	for i := firstIndex; i <= lastIndex; i++ {
		startOfNext := msfDuration(i+1, msf) // lastIndex+1 is the "leadout" track
		preemphasis := C.cdio_get_track_preemphasis(drive.d.p_cdio, C.uchar(i)) == C.CDIO_TRACK_FLAG_TRUE
		copyPermit := C.cdio_get_track_copy_permit(drive.d.p_cdio, C.uchar(i)) == C.CDIO_TRACK_FLAG_TRUE

		tracks = append(tracks,
			&Track{
				Num:        uint8(drive.d.disc_toc[i].bTrack),
				Duration:   startOfNext - startOfCurrent,
				Emphasis:   preemphasis,
				CopyPermit: copyPermit,
				drive:      drive,
			})

		startOfCurrent = startOfNext
	}

	return tracks
}

func (track *Track) offset() uint32 {
	s := C.cdio_cddap_track_firstsector(track.drive.d, C.uchar(track.Num))

	return uint32(s) + twoSecs
}

// returns the leadout track sector offset, 0 on error
func (drive *Drive) leadOutTrackOffset() uint32 {
	last := len(drive.Tracks) - 1

	s := C.cdio_cddap_track_firstsector(drive.d, C.uchar(drive.Tracks[last].Num+1)) // Num+1 is the "leadout" track
	if s == -1 {                                                                    // TODO: does it really return -1 on error?
		return 0
	}

	return uint32(s) + twoSecs
}

// Open the drive tray
func (drive *Drive) Open() {
	m.Lock()
	defer m.Unlock()
	if drive.d == nil {
		return
	}

	C.cdio_eject_media_drive(drive.d.cdda_device_name)
}

// Close the drive tray
func (drive *Drive) Close() {
	m.Lock()
	defer m.Unlock()
	if drive.d == nil { // drive obj got Destroy()ed while we waited for Lock()
		return
	}

	C.cdio_close_tray(drive.d.cdda_device_name, nil)
}

// NumOfTracks returns the number of numOfTracks on the drive
func (drive *Drive) NumOfTracks() uint8 {
	m.Lock()
	defer m.Unlock()
	if !drive.hasMedia /*disc removed*/ || drive.d == nil /*drive obj got Destroy()ed*/ {
		return 0
	}

	return drive.numOfTracks()
}

// Sectors returns the number of sectors on current disc. Returns 0 on error
func (drive *Drive) Sectors() uint32 {
	m.Lock()
	defer m.Unlock()
	if !drive.hasMedia /*disc removed*/ || drive.d == nil /*drive obj got Destroy()ed*/ {
		return 0
	}

	fs := int32(C.cdio_cddap_disc_firstsector(drive.d)) // typedef int32_t lsn_t
	ls := int32(C.cdio_cddap_disc_lastsector(drive.d))

	if fs == -1 || ls == -1 {
		return 0
	}

	return uint32(ls - fs + 1)
	// return drive.leadOutTrackOffset() - twoSecs // same
}

// same as NumOfTracks but without mutex
func (drive *Drive) numOfTracks() uint8 {
	if !drive.hasMedia {
		return 0
	}

	return (uint8)(C.cdio_cddap_tracks(drive.d))
}

// Destroy releases the drive object resources
func (drive *Drive) Destroy() {
	m.Lock()
	defer m.Unlock()

	if drive.d == nil {
		panic("attempt to Destroy() the same Drive consecutively")
	}

	delete(availableDrives, drive.Path)
	if len(availableDrives) == 0 { // whether we just removed the last remaining drive
		cb = nil
	}

	C.cdio_cddap_close(drive.d)

	drive.d = nil
	drive.Path = ""
	drive.Name = ""
	drive.Tracks = nil
}

// Destroy releases the drive object resources inside the Drives map
func (drives Drives) Destroy() {
	for _, d := range drives {
		d.Destroy()
	}
}
