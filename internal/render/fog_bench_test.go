package render

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/visibility"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// BenchmarkBuildFogOpsWindowLargeMap times the production fog build over a
// large map fog cache with a screen-sized window: the zoomed-out case, where
// the window covers a small part of a big map but the walk must not.
func BenchmarkBuildFogOpsWindowLargeMap(b *testing.B) {
	const gw, gh = 160, 160
	terr := &world.Terrain{CellW: gw * 2, CellH: gh * 2}
	svc := visibility.New(terr, visibility.ModeHistoryEnabled|visibility.ModeCurrentEnabled)
	if svc == nil {
		b.Fatal("visibility.New returned nil")
	}
	cache := svc.Fog()
	if cache == nil {
		b.Fatal("fog cache nil")
	}
	for row := int32(0); row < gh; row++ {
		for col := int32(0); col < gw; col++ {
			cache.SetChannel(col, row, uint8((col+row)%15+1), uint8((col*3+row)%15+1))
		}
	}
	cam := &camera.Camera{X: 2048, Z: 2048, ViewW: 1920, ViewH: 1080}
	var out []FogOp
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out = BuildFogOpsWindowWithArtInto(out, cache, cam, 1920, 1080, nil, false, [4]*formats.GAFEntry{}, [4]*formats.GAFEntry{})
	}
	_ = out
}
