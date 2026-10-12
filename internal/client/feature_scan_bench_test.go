package client

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
)

// BenchmarkScanFeatures times the one shared feature walk over a map-sized
// feature list with a screen-sized window. Before the shared scan, the
// short pass and pass A each paid a full walk of the same list per frame,
// and the retained light gather paid two more.
func BenchmarkScanFeatures(b *testing.B) {
	c := &Client{cam: &camera.Camera{ViewW: 1920, ViewH: 1080}}
	cur := &frame.Frame{Tick: 1}
	const n = 20000
	cur.Features = make([]frame.FeatureView, n)
	for i := range cur.Features {
		cur.Features[i].CX = int32(i % 200)
		cur.Features[i].CZ = int32(i / 200)
		if i%7 == 0 {
			cur.Features[i].Height = 12
		}
	}
	win := c.worldWindow()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.scanFeatures(cur, win)
	}
	if len(c.shortFeatures)+len(c.tallFeatures) == 0 {
		b.Fatal("scan admitted nothing")
	}
}
