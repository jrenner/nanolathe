package client

import (
	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	"github.com/nanolathe-gg/nanolathe/internal/render"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"testing"
)

// Enhanced artistic metadata: a hill reduces clearance, and water receives
// the shadow at sea level rather than at the submerged terrain (GPU design §34).
func TestAircraftShadowClearanceRefresh(t *testing.T) {
	c := reflectionScene(t) // detail scale, sea level 10
	g := &drawlist.ModelGeometry{Supersample: &drawlist.ModelGeometry{}}
	draw := &render.UnitDraw{Airborne: true, GroundY: 80 * numeric.FixedOne, WorldPos: [3]numeric.Fixed{0, 140 * numeric.FixedOne, 0}}
	for _, tc := range []struct {
		ground   int64
		airborne bool
		want     float32
	}{{80, true, 120}, {0, true, 260}, {0, false, 0}, {160, true, 0}} {
		draw.GroundY, draw.Airborne = numeric.Fixed(tc.ground)*numeric.FixedOne, tc.airborne
		c.setModelLightingHeight(g, draw)
		for _, p := range []*drawlist.ModelGeometry{g, g.Supersample, g.Clone()} {
			if p.AircraftShadowHeight != tc.want || p.AircraftShadowScale != 2 {
				t.Fatalf("ground=%d airborne=%v: height=%v scale=%v", tc.ground, tc.airborne, p.AircraftShadowHeight, p.AircraftShadowScale)
			}
		}
	}
	c.enhanced = false
	draw.Airborne = true
	c.setModelLightingHeight(g, draw)
	if g.AircraftShadowHeight != 0 {
		t.Fatal("classic acquired Enhanced shadow metadata")
	}
}

// Below lowDetailZoomCut the doubled supersample lane and the soft
// aircraft shadow stop being recorded; the model keeps its native
// corners and the ordinary silhouette route (§17, §34).
func TestLowDetailZoomDropsUnresolvableModelDetail(t *testing.T) {
	c := reflectionScene(t)
	c.antiAlias = true
	c.pal = &palette.Tables{}
	c.effects = drawlist.AllEffects()
	if !c.supersampleGeometry() {
		t.Fatal("detail view lost supersampled geometry")
	}
	g := &drawlist.ModelGeometry{}
	draw := &render.UnitDraw{Airborne: true, GroundY: 80 * numeric.FixedOne, WorldPos: [3]numeric.Fixed{0, 140 * numeric.FixedOne, 0}}
	c.cam.Zoom = lowDetailZoomCut
	if c.lowDetailZoom() || !c.supersampleGeometry() {
		t.Fatal("the cut itself is not low detail")
	}
	c.cam.Zoom = lowDetailZoomCut - 1
	if !c.lowDetailZoom() || c.supersampleGeometry() {
		t.Fatal("below the cut supersampled geometry is still recorded")
	}
	c.setModelLightingHeight(g, draw)
	if g.AircraftShadowHeight != 0 {
		t.Fatalf("below the cut aircraft clearance = %v, want the silhouette route", g.AircraftShadowHeight)
	}
	c.effects.Supersample = false
	c.cam.Zoom = camera.ZoomUnit
	if c.supersampleGeometry() {
		t.Fatal("the player's Supersample switch no longer wins at full zoom")
	}
}
