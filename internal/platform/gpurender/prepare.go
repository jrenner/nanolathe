package gpurender

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
)

// PrepareTerrain uploads the two terrain atlas scales at the loading boundary.
// It uses precisely the keys Execute will request: native omits detail art,
// detail includes the installed provider or nearest-doubles the native tiles.
// Repeated calls reuse the same pages; ResetSources owns their disposal.
// This moves one-time work and memory to loading, not a retail behavior change
// (DESIGN_GPU_RENDERER §14.8). Call only on the graphics-device owner.
func (r *Renderer) PrepareTerrain(sources drawlist.Terrain) {
	if r == nil {
		return
	}
	r.atlasFor(sources.Terrain, nil, camera.ViewScaleNative)
	r.atlasFor(sources.Terrain, sources.Detail, camera.ViewScaleDetail)
}

// PrepareSprites places sprite frames in the scene atlas at the loading
// boundary, by the same identity keys Execute uses, so a frame first seen
// mid-battle is already on its page. Placed on first use instead, the first
// sight of a stretch of map packs and uploads dozens of sprites inside one
// frame: in a traced game a camera jump onto unseen ground spent 6 ms in
// Replay, and the render thread then blocked 43 ms in one texture upload
// (Renderer.AtlasUploads now records how much atlas such a frame writes).
// Transient frames keep their own
// short-lived images. Like PrepareTerrain it moves one-time work to loading and
// changes no pixel (DESIGN_GPU_RENDERER §14.8). Call only on the
// graphics-device owner.
func (r *Renderer) PrepareSprites(frames []*formats.GAFFrame) {
	if r == nil {
		return
	}
	for _, f := range frames {
		if f != nil && !f.Transient {
			r.sceneFrameFor(f)
		}
	}
}

// PreparePrograms compiles the battle shader programs at the loading
// boundary. Programs are process-shared and compiled on first use
// (shader_cache.go); left lazy, each family's first appearance in a
// battle — the first explosion's glow, the first aircraft's soft shadow
// — pays a Kage compile inside that frame. Compiling here moves that
// one-time cost to loading, where PrepareTerrain and PrepareSprites
// already put the atlas uploads. Errors are not reported here: each
// family surfaces its own compile error at use exactly as it does today,
// and compileShader caches the outcome either way. Call only on the
// graphics-device owner.
func (r *Renderer) PreparePrograms() {
	if r == nil {
		return
	}
	for _, compile := range []func() (*ebiten.Shader, error){
		newScene2DShader, newSceneDestShader,
		newFogPassShader, newAircraftShadowShader, newArrivalShader,
		newDistortionShader, newGroundLightShader, newGroundResolveShader,
		newGroundClearShader, newLensShader, newMarkerShader, newScorchShader,
		newWaterShader, newStillWaterShader, newSurfaceWakeShader,
		newReflectionSourceShader, newReflectionResolveShader,
		newSoftReflectionResolveShader,
	} {
		_, _ = compile()
	}
	for _, src := range []string{
		fogOrderedShaderSource,
		glowSourceShaderSource(), glowShrinkShaderSource(),
		glowBlurShaderSource(), glowCompositeShaderSource(),
		modelDirectKeyShaderSource(), modelDirectColourShaderSource(),
		modelGroupMergeShaderSource, uploadFlushShaderSource,
	} {
		_, _ = compileShader(src)
	}
}

// SetSparseTerrain makes the tile atlases this renderer builds from now on
// fill on demand: a tile is packed and uploaded the first frame it is in view,
// onto pages of sparseTileAtlasSide² cells, instead of every tile of the map at
// its first sight. A view that holds still, like a settings preview, then
// uploads the few hundred tiles it shows rather than the whole map's thousands
// — tens of megabytes inside one frame at the detail scale. The pixels drawn
// are the same either way; only the cell a tile occupies differs. Battles keep
// the complete atlas PrepareTerrain uploads at loading, so a camera jump never
// packs tiles mid-game (DESIGN_GPU_RENDERER §14.8).
func (r *Renderer) SetSparseTerrain(on bool) {
	if r != nil {
		r.sparseTerrain = on
	}
}
