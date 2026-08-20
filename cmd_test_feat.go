//go:build ignore

package main

import (
	"fmt"
	"superstrike/internal/hidpp"
)

func main() {
	devs, _, err := hidpp.Discover()
	if err != nil || len(devs) == 0 {
		fmt.Println("no device:", err)
		return
	}
	d := devs[0]
	defer d.Close()
	feats, err := d.EnumerateFeatures()
	fmt.Println("EnumerateFeatures err:", err)
	fmt.Println("count:", len(feats))
	for _, f := range feats {
		fmt.Printf("  idx 0x%02X  id 0x%04X  %s\n", f.Index, f.ID, f.Name())
	}
}
