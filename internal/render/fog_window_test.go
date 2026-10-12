package render

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	"github.com/nanolathe-gg/nanolathe/internal/visibility"
)

// buildFogOpsWindowIntoNoArt is the windowed walk with no authored art extents,
// the shape BuildFogOpsWindowWithArtInto takes when a family resolves nothing.
func buildFogOpsWindowIntoNoArt(out []FogOp, cache *visibility.FogCache, cam *camera.Camera, surfW, surfH int32, tables *palette.Tables, dither bool) []FogOp {
	return buildFogOpsWindowInto(out, cache, cam, surfW, surfH, tables, dither, nil)
}

// TestFogWindowMatchesClippedFullBuild locks the windowed builder against the
// map-wide builder: without art extents every GAF op must survive, while the
// fills are culled to the surface, in the same order.
//
// An unbounded GAF frame may cross any nominal cell edge. Fills alone use the
// nominal rectangle; clipping a GAF is the executor's job once art is known.
func TestFogWindowMatchesClippedFullBuild(t *testing.T) {
	const gw, gh = 24, 24
	cache := testFogCache(t, gw, gh)
	// A mix of never-seen, fogged-but-explored and GAF-valued cells, so the
	// builder emits every operation kind including the cells that emit two.
	for row := int32(0); row < gh; row++ {
		for col := int32(0); col < gw; col++ {
			c0 := uint8((col*3 + row*5) % 16)
			c1 := uint8((col*7 + row*11) % 16)
			cache.SetChannel(col, row, c0, c1)
		}
	}

	surfW, surfH := int32(320), int32(200)
	// Cameras inside the grid, off each edge, and at negative and non-tile
	// -aligned positions: the window's row and column tests are the part that
	// can be off by a tile.
	for _, cam := range []*camera.Camera{
		{X: 0, Z: 0, ViewW: surfW, ViewH: surfH},
		{X: 17, Z: 3, ViewW: surfW, ViewH: surfH},
		{X: 200, Z: 150, ViewW: surfW, ViewH: surfH},
		{X: -64, Z: -48, ViewW: surfW, ViewH: surfH},
		{X: -1, Z: -1, ViewW: surfW, ViewH: surfH},
		{X: 511, Z: 511, ViewW: surfW, ViewH: surfH},
		{X: 5000, Z: 5000, ViewW: surfW, ViewH: surfH},
	} {
		for _, dither := range []bool{false, true} {
			full := buildFogOpsMapWide(nil, cache, cam, nil, dither)
			var want []FogOp
			for _, op := range full {
				x0 := op.ScreenX0 - camera.OriginX
				y0 := op.ScreenY0 - camera.OriginY
				x1 := op.ScreenX1 - camera.OriginX
				y1 := op.ScreenY1 - camera.OriginY
				if x0 < 0 {
					x0 = 0
				}
				if y0 < 0 {
					y0 = 0
				}
				if x1 > surfW {
					x1 = surfW
				}
				if y1 > surfH {
					y1 = surfH
				}
				if (x0 >= x1 || y0 >= y1) && op.Kind != FogKindGAFCh0 && op.Kind != FogKindGAFCh1 {
					continue
				}
				want = append(want, op)
			}
			got := buildFogOpsWindowIntoNoArt(nil, cache, cam, surfW, surfH, nil, dither)
			if len(got) != len(want) {
				t.Fatalf("camera (%d,%d) dither=%v: windowed build has %d ops, the clipped map-wide build has %d",
					cam.X, cam.Z, dither, len(got), len(want))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("camera (%d,%d) dither=%v op %d: windowed %+v, clipped map-wide %+v",
						cam.X, cam.Z, dither, i, got[i], want[i])
				}
			}
		}
	}
}

// TestFogWindowRejectsEmptySurface checks the degenerate surface produces no
// operations rather than the whole map: a zero-sized surface clips everything
// away, and the windowed builder must agree.
func TestFogWindowRejectsEmptySurface(t *testing.T) {
	cache := testFogCache(t, 4, 4)
	cache.SetChannel(0, 0, 15, 0)
	cam := &camera.Camera{X: 0, Z: 0, ViewW: 320, ViewH: 200}
	for _, size := range [][2]int32{{0, 200}, {320, 0}, {-1, -1}} {
		if ops := buildFogOpsWindowIntoNoArt(nil, cache, cam, size[0], size[1], nil, false); len(ops) != 0 {
			t.Fatalf("surface %dx%d produced %d ops, want none", size[0], size[1], len(ops))
		}
	}
}

// TestFogWindowReusesScratch checks the scratch slice is reused rather than
// reallocated: the composer hands the same slice back every frame and relies on
// it keeping its capacity.
func TestFogWindowReusesScratch(t *testing.T) {
	cache := testFogCache(t, 8, 8)
	for row := int32(0); row < 8; row++ {
		for col := int32(0); col < 8; col++ {
			cache.SetChannel(col, row, 15, 0)
		}
	}
	cam := &camera.Camera{X: 0, Z: 0, ViewW: 320, ViewH: 200}
	scratch := buildFogOpsWindowIntoNoArt(nil, cache, cam, 320, 200, nil, false)
	if len(scratch) == 0 {
		t.Fatal("scene produces no fog ops; the reuse check would prove nothing")
	}
	grown := scratch[:cap(scratch)]
	again := buildFogOpsWindowIntoNoArt(scratch, cache, cam, 320, 200, nil, false)
	if cap(again) != cap(grown) || &again[:1][0] != &grown[:1][0] {
		t.Fatal("windowed build reallocated the scratch slice instead of refilling it")
	}
}

// With authored extents the bounded walk must retain the same ordered draws as
// the map-wide walk, including a one-pixel overhang beyond a nominal cell.
func TestFogWindowWithArtMatchesFullBuild(t *testing.T) {
	cache := testFogCache(t, 24, 24)
	for y := int32(0); y < 24; y++ {
		for x := int32(0); x < 24; x++ {
			cache.SetChannel(x, y, uint8((x*3+y*5)%16), uint8((x*7+y*11)%16))
		}
	}
	fr := &formats.GAFFrame{Width: 33, Height: 33}
	entry := &formats.GAFEntry{Frames: make([]formats.GAFFrameRef, 14)}
	for i := range entry.Frames {
		entry.Frames[i].Frame = fr
	}
	family := [4]*formats.GAFEntry{entry, entry, entry, entry}
	for _, scale := range []camera.ViewScale{camera.ViewScaleNative, camera.ViewScaleDetail} {
		for _, pan := range [][2]int32{{48, 48}, {17, 3}, {511, 511}, {5000, 5000}, {-32, -32}} {
			cam := &camera.Camera{X: pan[0], Z: pan[1], Scale: scale}
			full := buildFogOpsMapWide(nil, cache, cam, nil, false)
			var want []FogOp
			for _, op := range full {
				edge := scale.Px(32)
				if op.Kind == FogKindGAFCh0 || op.Kind == FogKindGAFCh1 {
					edge = scale.Px(33)
				}
				x, y := op.ScreenX0-camera.OriginX, op.ScreenY0-camera.OriginY
				if x+edge > 0 && y+edge > 0 && x < 64 && y < 64 {
					want = append(want, op)
				}
			}
			got := BuildFogOpsWindowWithArtInto(nil, cache, cam, 64, 64, nil, false, family, family)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("scale %d camera %v: got %d ops, want %d ordered ops", scale, pan, len(got), len(want))
			}
		}
	}
}

// TestFogAxisRangeMatchesCellTest locks the range the bounded walk visits
// against the per-cell surface test it replaces: exactly the cells passing
// the test, in order, for either scale and cameras on, off and far off the
// grid.
func TestFogAxisRangeMatchesCellTest(t *testing.T) {
	for _, scale := range []camera.ViewScale{camera.ViewScaleNative, camera.ViewScaleDetail} {
		for _, tc := range []struct {
			origin, camPos, count, surf, lo, hi int32
		}{
			{0, 0, 160, 1080, 0, 32},
			{0, 2048, 160, 1920, -20, 52},
			{0, -64, 24, 200, -33, 65},
			{3, 5000, 24, 64, 0, 32},
			{0, -5000, 24, 64, -100, 100},
			{5, 17, 1, 1, 0, 32},
			{0, 31, 160, 33, 31, 63},
		} {
			lo, hi := fogAxisRange(tc.origin, tc.camPos, tc.count, tc.surf, tc.lo, tc.hi, scale)
			var wantLo, wantHi int32 = -1, -1
			for i := int32(0); i < tc.count; i++ {
				p := scale.Project((tc.origin+i)*FogTilePixels + FogTilePixels/2 - tc.camPos)
				if p+tc.hi > 0 && p+tc.lo < tc.surf {
					if wantLo < 0 {
						wantLo = i
					}
					wantHi = i + 1
				}
			}
			if wantLo < 0 {
				wantLo, wantHi = lo, lo // empty: only emptiness is checked below
				if lo != hi {
					t.Fatalf("scale %d %+v: got range [%d,%d), want empty", scale, tc, lo, hi)
				}
				continue
			}
			if lo != wantLo || hi != wantHi {
				t.Fatalf("scale %d %+v: got range [%d,%d), want [%d,%d)", scale, tc, lo, hi, wantLo, wantHi)
			}
		}
	}
}
