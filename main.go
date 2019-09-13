package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"main/metadata"
	"main/odd"
)

func main() {
	drives := odd.Get(func(path string, media bool) {
		if media {
			fmt.Printf("inserted %s\n", path)
		} else {
			fmt.Printf("ejected %s\n", path)
		}
	})

	if drives == nil {
		fmt.Println("No drives found")
		os.Exit(1)
	}

	fmt.Printf("%d drives found:\n", len(drives))

	for _, d := range drives {
		fmt.Printf("name: %s\n", d.Name)
		fmt.Printf("path: %s\n", d.Path)
		fmt.Printf("tracks: %d\n", d.NumOfTracks())
		fmt.Printf("sectors: %d\n", d.Sectors())

		discid, err := d.CalcDiscID()
		albums, _ := metadata.QueryMusicBrainz(discid)
		if err != nil {
			fmt.Printf("umm no metadata: %s\n", err)
		}

		data, err := json.MarshalIndent(albums, "", "  ")
		if err != nil {
			log.Fatalf("JSON marshaling failed: %s", err)
		}
		fmt.Printf("%s\n", data)
	}

	drives.Destroy()
}
