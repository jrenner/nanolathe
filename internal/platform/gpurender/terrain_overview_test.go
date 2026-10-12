package gpurender

import (
	"fmt"
	"os"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
)

// TestOverviewChunkPixelsDecimates locks the overview build: every chunk
// texel is the native tile pixel at twice its coordinate, never an average
// of palette indices, and the chunk is opaque.
func TestOverviewChunkPixelsDecimates(t *testing.T) {
	terrain := terrainFixtureTerrain()
	buf, w, h := overviewChunkPixels(terrain, 2, 1, 0, 0)
	if w != 32 || h != 16 {
		t.Fatalf("chunk size = %dx%d, want 32x16", w, h)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			id := (2 * x) / terrainTileSize
			want := terrain.TileSet[id][(2*y)*terrainTileSize+(2*x)%terrainTileSize]
			at := (y*w + x) * 4
			if buf[at] != want || buf[at+3] != 255 {
				t.Fatalf("texel (%d,%d) = (%d,%d), want (%d,255)", x, y, buf[at], buf[at+3], want)
			}
		}
	}
}

// TestOverviewActiveGate locks which factors take the overview: the native
// record scale at or below the strategic cut, nothing else.
func TestOverviewActiveGate(t *testing.T) {
	r := &Renderer{}
	for _, tc := range []struct {
		zoom  camera.Zoom
		scale camera.ViewScale
		want  bool
	}{
		{camera.ZoomUnit / 2, camera.ViewScaleNative, true},
		{camera.ZoomUnit / 4, camera.ViewScaleNative, true},
		{camera.ZoomUnit/2 + 1, camera.ViewScaleNative, false},
		{camera.ZoomUnit, camera.ViewScaleNative, false},
		{0, camera.ViewScaleNative, false},
		{camera.ZoomUnit / 2, camera.ViewScaleDetail, false},
	} {
		r.worldZoom = tc.zoom
		if got := r.overviewActive(tc.scale); got != tc.want {
			t.Errorf("zoom %d scale %d: overviewActive = %v, want %v", tc.zoom, tc.scale, got, tc.want)
		}
	}
}

// checkOverviewDevicePixels draws the fixture map through the overview at
// exactly half zoom and checks every output pixel against the native tile
// pixel the decimation and the half-scale sampling both select.
func checkOverviewDevicePixels() error {
	pal := fixturePalette()
	terrain := terrainFixtureTerrain()
	cam := &camera.Camera{X: 0, Z: 0, Scale: camera.ViewScaleNative}
	const w, h = 32, 16 // the 64x32 map at half scale
	r, err := NewChecked(&pal, w, h)
	if err != nil {
		return err
	}
	var list drawlist.List
	list.RecordClear()
	list.RecordWorld(worldRegion(camera.ZoomUnit/2, camera.ViewScaleNative, 64, 32))
	list.RecordTerrain(drawlist.Terrain{
		Terrain: terrain, Cam: cam,
		OriginX: 0, OriginY: 0,
		DstW: 64, DstH: 32,
		Scale: camera.ViewScaleNative,
	})
	list.RecordWorld(drawlist.WorldSpace{Begin: false})
	list.RecordExpand()
	img := r.Execute(&list, w, h)
	if img == nil {
		return fmt.Errorf("overview device fixture returned no image")
	}
	if len(r.overviews) != 1 {
		return fmt.Errorf("overviews built = %d, want 1 (the overview path did not run)", len(r.overviews))
	}
	pixels := make([]byte, w*h*4)
	img.ReadPixels(pixels)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			wx, wy := 2*x, 2*y
			id := wx / terrainTileSize
			want := terrain.TileSet[id][wy*terrainTileSize+wx%terrainTileSize]
			if err := checkExactIndex(fmt.Sprintf("overview at (%d,%d)", x, y), pixels, (y*w+x)*4, &pal, want); err != nil {
				return err
			}
		}
	}
	return nil
}

// TestOverviewDeviceFixture is opt-in like the other device fixtures
// (C-G10); the hidden loop in TestMain runs checkOverviewDevicePixels.
func TestOverviewDeviceFixture(t *testing.T) {
	if os.Getenv("NANOLATHE_GPU_DEVICE_TEST") != "1" {
		t.Skip("set NANOLATHE_GPU_DEVICE_TEST=1 for real-device overview fixtures")
	}
	if deviceFixtureResult != nil {
		t.Fatalf("device fixture loop: %v", deviceFixtureResult)
	}
}
