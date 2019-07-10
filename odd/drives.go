package odd

// *Optical Disc Drive

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

uint8_t countDrives(char **drives);
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
	Drive struct {
		Path string
		Name string

		d     *C.cdrom_drive_t
		media bool // whether an Audio CD is currently present in drive. yes, there's cdrom_drive_t.opened...
	}

	Track struct {
		Num        uint8
		Duration   time.Duration
		Emphasis   bool
		CopyPermit bool
	}

	// Drives is a simple map of Drive struct pointers, keyed with drive paths
	Drives map[string]*Drive

	Tracks []*Track
)

var (
	// prevents g_main_loop racing against caller
	m sync.Mutex
	// Media change callbacks
	cb func(string, bool)

	// contains all drives returned with the last Get()
	returnedDrives Drives
)

//export VolumeRemovedCB
func VolumeRemovedCB(_ *C.GVolumeMonitor, v *C.GVolume, _ C.gpointer) {
	m.Lock()
	defer m.Unlock()

	path := gvolume2Path(v)

	if drive, found := returnedDrives[path]; found && cb != nil {
		drive.media = false
		cb(path, false)
	}
}

//export VolumeAddedCB
func VolumeAddedCB(_ *C.GVolumeMonitor, v *C.GVolume, _ C.gpointer) {
	m.Lock()
	defer m.Unlock()

	path := gvolume2Path(v)

	if drive, found := returnedDrives[path]; found {
		if drive.initMedia() && /*must be last*/ cb != nil {
			cb(path, true)
		}
	}
}

func init() {
	returnedDrives = make(Drives)

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
// onChange() is called when an audio CD is either removed or inserted.
// media == true indicates media changed into valid Audio CD
// onChange() must not call any methods on any Drive object directly
func Get(mediaChange func(path string, media bool)) Drives {
	m.Lock()
	defer m.Unlock()

	if len(returnedDrives) != 0 {
		panic("Every Get() call must be preceded with Destroy()ction for all Get()ed drives")
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
		returnedDrives[path] = drives[path]
		cb = mediaChange

		// parallelize initMedia() 4 fun
		wg.Add(1)
		go func() {
			defer wg.Done()

			returnedDrives[path].initMedia()
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
	return (*[0xff]*C.char)(unsafe.Pointer(cArray))[:deviceCount:deviceCount /*capacity*/ ]
}

func gvolume2Path(volume *C.GVolume) string {
	return C.GoString(C.g_volume_get_identifier(volume, C.CString(C.G_DRIVE_IDENTIFIER_KIND_UNIX_DEVICE))) // deprecated, whatever
}

// cdparanoia verifies for us whether the inserted disc is CDDA
func (drive *Drive) initMedia() bool {
	drive.media = C.cdio_cddap_open(drive.d) == 0

	return drive.media
}

// Tracks returns a slice of Track structs
func (drive *Drive) Tracks() Tracks {
	msfDuration := func(i uint8, msf *C.msf_t) time.Duration {
		deBCD := func(b C.uchar) time.Duration {
			return time.Duration(((b&0xF0)>>4)*10 + (b & 0x0F))
		}

		if !C.cdio_get_track_msf(drive.d.p_cdio, drive.d.disc_toc[i].bTrack, msf) {
			return 0
		}

		minutes := deBCD(msf.m) // readability ftw
		seconds := deBCD(msf.s)
		frames := deBCD(msf.f)

		return time.Minute * minutes + time.Second * seconds + time.Second / 75 * frames
	}

	m.Lock()
	defer m.Unlock()

	totalTracks := drive.numOfTracks()
	if totalTracks == 0 {
		return nil
	}

	first := uint8(C.cdio_get_first_track_num(drive.d.p_cdio))
	firstIndex := 1 - first
	lastIndex := totalTracks - first

	var tracks Tracks
	msf := new(C.msf_t)
	startOfCurrent := msfDuration(firstIndex, msf)
	for i := firstIndex; i <= lastIndex; i++ {
		startOfNext := msfDuration(i+1, msf) // last_track+1 is the "leadout" track
		preemphasis := C.cdio_get_track_preemphasis(drive.d.p_cdio, C.uchar(i)) == C.CDIO_TRACK_FLAG_TRUE
		copyPermit := C.cdio_get_track_copy_permit(drive.d.p_cdio, C.uchar(i)) == C.CDIO_TRACK_FLAG_TRUE

		tracks = append(tracks,
			&Track{
				Num:        uint8(drive.d.disc_toc[i].bTrack),
				Duration:   startOfNext - startOfCurrent,
				Emphasis:   preemphasis,
				CopyPermit: copyPermit,
			})

		startOfCurrent = startOfNext
	}

	return tracks
}

// Open the drive tray
func (drive *Drive) Open() {
	m.Lock()
	defer m.Unlock()

	C.cdio_eject_media_drive(drive.d.cdda_device_name)
}

// Close the drive tray
func (drive *Drive) Close() {
	m.Lock()
	defer m.Unlock()

	C.cdio_close_tray(drive.d.cdda_device_name, nil)
}

// NumOfTracks returns the number of numOfTracks on the drive
func (drive *Drive) NumOfTracks() uint8 {
	m.Lock()
	defer m.Unlock()

	return drive.numOfTracks()
}

// same as NumOfTracks but without mutex
func (drive *Drive) numOfTracks() uint8 {
	if !drive.media {
		return 0
	}

	return (uint8)(C.cdio_cddap_tracks(drive.d))
}

// Destroy releases the drive object resources
func (drive *Drive) Destroy() {
	m.Lock()
	defer m.Unlock()

	delete(returnedDrives, drive.Path)
	if len(returnedDrives) == 0 {
		cb = nil
	}

	C.cdio_cddap_close(drive.d)

	drive.d = nil
	drive.Path = ""
	drive.Name = ""
}

// Destroy releases the drive object resources inside the Drives map
func (drives Drives) Destroy() {
	for _, d := range drives {
		d.Destroy()
	}
}
