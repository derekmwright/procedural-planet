//go:build ignore

// Compare matched screenshots without requiring an imaging package.
// Usage: go run tools/compare_frames.go before.png after.png
package main

import (
	"fmt"
	"image"
	_ "image/png"
	"math"
	"os"
)

func readFrame(path string) image.Image {
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	im, _, err := image.Decode(f)
	if err != nil {
		panic(err)
	}
	return im
}

func main() {
	if len(os.Args) != 3 {
		panic("usage: compare_frames before.png after.png")
	}
	a, b := readFrame(os.Args[1]), readFrame(os.Args[2])
	if a.Bounds() != b.Bounds() {
		panic("image dimensions differ")
	}
	var histogram [256]int64
	var sum, square, count int64
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			ar, ag, ab, _ := a.At(x, y).RGBA()
			br, bg, bb, _ := b.At(x, y).RGBA()
			for _, d := range []int{int(ar>>8) - int(br>>8), int(ag>>8) - int(bg>>8), int(ab>>8) - int(bb>>8)} {
				if d < 0 {
					d = -d
				}
				histogram[d]++
				sum += int64(d)
				square += int64(d * d)
				count++
			}
		}
	}
	var cumulative int64
	var p99, maxDiff int
	for d, n := range histogram {
		if n > 0 {
			maxDiff = d
		}
		if cumulative < (count*99+99)/100 {
			p99 = d
		}
		cumulative += n
	}
	fmt.Printf("mean absolute %.6f; RMS %.6f; p99 %d; max %d (8-bit RGB, out of 255); identical %.4f%%\n",
		float64(sum)/float64(count), math.Sqrt(float64(square)/float64(count)), p99, maxDiff, 100*float64(histogram[0])/float64(count))
}
