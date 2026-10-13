package gpurender

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
)

// Front-loaded uploads (DESIGN_GPU_RENDERER §11.5 "Uploads come first").
//
// Ebitengine holds an image's WritePixels until the first draw that reads or
// writes the image, and only then queues the upload, right before that draw.
// Its OpenGL backend then finishes every earlier draw on the device before any
// upload that follows a draw. So the minimap's rewrite, uploaded where the
// HUD first drew it, held the render thread until the GPU had drawn the whole
// world and the model lane's page passes queued before it; the GPU then sat
// idle while the next frame was built, and a frame cost the CPU's time plus
// the GPU's. In the 3,000-unit field battle at 2x on an integrated GPU that
// wait was about 21 ms of a 49 ms frame.
//
// uploadFrameSources writes the textures a frame is known to rewrite before
// anything draws — the identified surfaces, the fog grid and the sprite frames
// the list names — and flushFrameUploads then reads every image written so far
// in one or two one-pixel draws, so all of their uploads are queued together
// ahead of the frame's first real draw. The one device wait left is the first
// upload's, for the previous frame, which the GPU ran while this one was being
// built; a later upload waits only for those one-pixel draws. Uploads the list
// does not name in advance (a lit point plane, a new flash disc) keep their
// place. No pixel changes: each source is written once, with the same bytes,
// before the draws that read it.

// uploadFlushShaderSource writes nothing a frame reads; the draw exists only
// to name its sources.
const uploadFlushShaderSource = `//kage:unit pixels

package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	return vec4(0)
}
`

// uploadFrameSources writes the sources this frame is known to rewrite before
// the frame's first draw: identified surfaces (the minimap and the megamap),
// the fog grid and the sprite frames the list names. Replay looks each of them
// up again and finds it written; a surface without an identity keeps its
// upload at replay, and a fog grid whose clip differs there is uploaded again.
func (r *Renderer) uploadFrameSources(list *drawlist.List) {
	list.VisitSurfaces(func(sf drawlist.Surface) {
		if sf.Identity != 0 && len(sf.Pixels) != 0 {
			r.uploadSurface(sf)
		}
	})
	recordW, recordH := list.RecordedWorldExtent()
	list.VisitFog(func(fg drawlist.Fog) { r.prepareFogGrid(fg, recordW, recordH) })
	list.VisitSprites(func(sp drawlist.Sprite) {
		if spriteUsesSceneFrame(sp) {
			r.sceneFrameFor(sp.Frame)
		}
	})
}

// spriteUsesSceneFrame reports whether Sprite's replay of sp places sp.Frame
// in the scene atlas: every kind does, except a PCX background and a raw
// glyph strip, which have atlases of their own.
func spriteUsesSceneFrame(sp drawlist.Sprite) bool {
	if sp.PCX != nil || sp.Frame == nil {
		return false
	}
	switch sp.Kind {
	case drawlist.BlitKeyed, drawlist.BlitScaled, drawlist.BlitFeatureNormal, drawlist.BlitTinted, drawlist.BlitFeatureShadow:
		return true
	case drawlist.BlitLit:
		return sp.Pal != nil && sp.Frame.Compressed != 0
	}
	return false
}

// noteUpload records an image this frame wrote for flushFrameUploads.
func (r *Renderer) noteUpload(img *ebiten.Image) {
	if img != nil {
		r.uploaded = append(r.uploaded, img)
	}
}

// flushFrameUploads queues the uploads of every image written since the frame
// began, ahead of any draw that follows, and forgets them.
func (r *Renderer) flushFrameUploads() {
	imgs := append(r.uploaded, r.scene.written...)
	r.uploaded, r.scene.written = r.uploaded[:0], r.scene.written[:0]
	if len(imgs) == 0 {
		return
	}
	shader, err := compileShader(uploadFlushShaderSource)
	if err != nil || shader == nil {
		return
	}
	if r.uploadTarget == nil {
		r.uploadTarget = newRendererImage(1, 1)
		// One triangle over the target's single pixel centre.
		r.uploadFlushVerts = [3]ebiten.Vertex{{DstX: 0, DstY: 0}, {DstX: 2, DstY: 0}, {DstX: 0, DstY: 2}}
	}
	var group [4]*ebiten.Image
	n := 0
	flush := func() {
		if n == 0 {
			return
		}
		r.uploadFlushOpts.Images = group
		r.uploadTarget.DrawTrianglesShader32(r.uploadFlushVerts[:], uploadFlushIndices[:], shader, &r.uploadFlushOpts)
		group, n = [4]*ebiten.Image{}, 0
	}
	for i, img := range imgs {
		seen := false
		for _, earlier := range imgs[:i] {
			if earlier == img {
				seen = true
				break
			}
		}
		if seen {
			continue
		}
		group[n] = img
		n++
		if n == len(group) {
			flush()
		}
	}
	flush()
}

// uploadFlushIndices is the one triangle of a flush draw, which covers the
// target's single pixel.
var uploadFlushIndices = [3]uint32{0, 1, 2}
