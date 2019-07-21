package main

import (
	"fmt"
	"os"

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
		fmt.Printf("DiscID: %s\n", d.CalcDiscID())

		// d.Open()
		// time.Sleep(time.Second * 2)
		// d.Close()
		// time.Sleep(time.Second * 5)
		// fmt.Println()
	}

	// for {
	// 	fmt.Printf("tracks0 %d\n", drives["/dev/sr0"].NumOfTracks())
	// 	fmt.Printf("tracks1 %d\n", drives["/dev/sr1"].NumOfTracks())
	// 	time.Sleep(time.Second)
	// }

	tracks := drives["/dev/sr0"].Tracks
	if tracks == nil {
		fmt.Println("umm no tracks")
	}

	for _, t := range tracks {
		fmt.Printf("%+v\n", t) // %+v
	}

	// tracks = drives["/dev/sr1"].Tracks
	// if tracks == nil {
	// 	fmt.Println("umm no tracks")
	// }
	//
	// for _, t := range tracks {
	// 	fmt.Printf("%+v\n", t)
	// }

	drives.Destroy()
}
