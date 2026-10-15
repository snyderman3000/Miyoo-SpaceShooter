package main

// 2D drawing on a 0xAARRGGBB canvas (the Miyoo framebuffer's own pixel
// format, so presenting is a plain copy) plus cached TrueType text.
// Palette and fonts are Mixtape's: amber phosphor with cyan/magenta accents.

import (
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

//go:embed assets/body.ttf
var bodyTTF []byte

//go:embed assets/title.ttf
var titleTTF []byte

//go:embed assets/lcd.ttf
var lcdTTF []byte

var (
	colBG       = rgb(0x07, 0x08, 0x0a)
	colPanel    = rgb(0x10, 0x12, 0x15)
	colPanel2   = rgb(0x17, 0x1a, 0x1e)
	colGrid     = rgb(0x22, 0x26, 0x2c)
	colAmber    = rgb(0xff, 0xb0, 0x00)
	colAmberDim = rgb(0x8a, 0x5e, 0x00)
	colAmberDk  = rgb(0x3a, 0x28, 0x00)
	colCyan     = rgb(0x2e, 0xe6, 0xd6)
	colCyanDim  = rgb(0x12, 0x5e, 0x58)
	colMagenta  = rgb(0xff, 0x2e, 0x88)
	colMagDim   = rgb(0x5c, 0x10, 0x31)
	colRed      = rgb(0xff, 0x3b, 0x30)
	colGreen    = rgb(0x7d, 0xff, 0x6a)
	colPaper    = rgb(0xe8, 0xe2, 0xd0)
	colPaperDim = rgb(0x8c, 0x88, 0x7c)
	colWhite    = rgb(0xff, 0xff, 0xff)
)

func rgb(r, g, b uint8) uint32 { return 0xff000000 | uint32(r)<<16 | uint32(g)<<8 | uint32(b) }

func chans(c uint32) (int, int, int) { return int(c>>16) & 255, int(c>>8) & 255, int(c) & 255 }

func mix(a, b uint32, t float64) uint32 {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	ar, ag, ab := chans(a)
	br, bg, bb := chans(b)
	k := int(t * 256)
	return 0xff000000 | uint32(ar+((br-ar)*k>>8))<<16 | uint32(ag+((bg-ag)*k>>8))<<8 | uint32(ab+((bb-ab)*k>>8))
}

// scale multiplies a colour's brightness by k (0..~1.5), clamped.
func scale(c uint32, k float64) uint32 {
	r, g, b := chans(c)
	cl := func(v int) uint32 {
		x := int(float64(v) * k)
		if x > 255 {
			x = 255
		}
		if x < 0 {
			x = 0
		}
		return uint32(x)
	}
	return 0xff000000 | cl(r)<<16 | cl(g)<<8 | cl(b)
}

type Canvas struct {
	W, H int
	Pix  []uint32
}

func NewCanvas(w, h int) *Canvas { return &Canvas{W: w, H: h, Pix: make([]uint32, w*h)} }

func (c *Canvas) clip(x, y, w, h int) (int, int, int, int, bool) {
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > c.W {
		w = c.W - x
	}
	if y+h > c.H {
		h = c.H - y
	}
	return x, y, w, h, w > 0 && h > 0
}

func (c *Canvas) Fill(x, y, w, h int, col uint32) {
	x, y, w, h, ok := c.clip(x, y, w, h)
	if !ok {
		return
	}
	for yy := y; yy < y+h; yy++ {
		row := c.Pix[yy*c.W+x : yy*c.W+x+w]
		for i := range row {
			row[i] = col
		}
	}
}

// Blend paints col over the rectangle with opacity a (0..1).
func (c *Canvas) Blend(x, y, w, h int, col uint32, a float64) {
	x, y, w, h, ok := c.clip(x, y, w, h)
	if !ok {
		return
	}
	k := int(a * 256)
	cr, cg, cb := chans(col)
	for yy := y; yy < y+h; yy++ {
		row := c.Pix[yy*c.W+x : yy*c.W+x+w]
		for i, p := range row {
			r, g, b := chans(p)
			row[i] = 0xff000000 | uint32(r+((cr-r)*k>>8))<<16 | uint32(g+((cg-g)*k>>8))<<8 | uint32(b+((cb-b)*k>>8))
		}
	}
}

func (c *Canvas) Set(x, y int, col uint32) {
	if x >= 0 && y >= 0 && x < c.W && y < c.H {
		c.Pix[y*c.W+x] = col
	}
}

func (c *Canvas) Border(x, y, w, h, t int, col uint32) {
	c.Fill(x, y, w, t, col)
	c.Fill(x, y+h-t, w, t, col)
	c.Fill(x, y, t, h, col)
	c.Fill(x+w-t, y, t, h, col)
}

// Corners draws Mixtape's HUD corner brackets.
func (c *Canvas) Corners(x, y, w, h, l, t int, col uint32) {
	c.Fill(x, y, l, t, col)
	c.Fill(x, y, t, l, col)
	c.Fill(x+w-l, y, l, t, col)
	c.Fill(x+w-t, y, t, l, col)
	c.Fill(x, y+h-t, l, t, col)
	c.Fill(x, y+h-l, t, l, col)
	c.Fill(x+w-l, y+h-t, l, t, col)
	c.Fill(x+w-t, y+h-l, t, l, col)
}

// Stripe is Mixtape's amber/magenta/cyan tape stripe.
func (c *Canvas) Stripe(x, y, w, t int) {
	c.Fill(x, y, w, t, colAmber)
	c.Fill(x, y+t, w, t, colMagenta)
	c.Fill(x, y+2*t, w, t, colCyan)
}

func (c *Canvas) Line(x0, y0, x1, y1 int, col uint32) {
	dx := iabs(x1 - x0)
	dy := -iabs(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	// reject lines entirely off screen
	if (x0 < 0 && x1 < 0) || (y0 < 0 && y1 < 0) || (x0 >= c.W && x1 >= c.W) || (y0 >= c.H && y1 >= c.H) {
		return
	}
	err := dx + dy
	for n := 0; n < 4000; n++ {
		c.Set(x0, y0, col)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func (c *Canvas) Circle(cx, cy, r int, col uint32, filled bool, thick int) {
	r2 := r * r
	in := (r - thick) * (r - thick)
	for y := -r; y <= r; y++ {
		py := cy + y
		if py < 0 || py >= c.H {
			continue
		}
		for x := -r; x <= r; x++ {
			d := x*x + y*y
			if d <= r2 && (filled || d >= in) {
				c.Set(cx+x, py, col)
			}
		}
	}
}

func iabs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Upscale2x draws src at twice its size, darkening odd rows for a CRT look.
func (c *Canvas) Upscale2x(src *Canvas) {
	w := src.W
	if w*2 > c.W {
		w = c.W / 2
	}
	h := src.H
	if h*2 > c.H {
		h = c.H / 2
	}
	for y := 0; y < h; y++ {
		s := src.Pix[y*src.W : y*src.W+w]
		d0 := c.Pix[(2*y)*c.W : (2*y)*c.W+2*w]
		d1 := c.Pix[(2*y+1)*c.W : (2*y+1)*c.W+2*w]
		for x, p := range s {
			d0[2*x] = p
			d0[2*x+1] = p
			q := p - (p>>2)&0x003f3f3f
			d1[2*x] = q
			d1[2*x+1] = q
		}
	}
}

func (c *Canvas) ToImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, c.W, c.H))
	for i, p := range c.Pix {
		img.Pix[i*4] = uint8(p >> 16)
		img.Pix[i*4+1] = uint8(p >> 8)
		img.Pix[i*4+2] = uint8(p)
		img.Pix[i*4+3] = 255
	}
	return img
}

// ---------- text ----------

type glyph struct {
	mask    *image.Alpha
	off     image.Point
	advance int
}

type Font struct {
	face   font.Face
	cache  map[rune]*glyph
	ascent int
	height int
}

func loadFont(ttf []byte, size float64) *Font {
	f, err := opentype.Parse(ttf)
	if err != nil {
		panic(err)
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	m := face.Metrics()
	return &Font{face: face, cache: map[rune]*glyph{}, ascent: m.Ascent.Ceil(), height: m.Height.Ceil()}
}

func (f *Font) glyph(r rune) *glyph {
	if g, ok := f.cache[r]; ok {
		return g
	}
	dr, mask, mp, adv, ok := f.face.Glyph(fixed.P(0, 0), r)
	if !ok {
		g := &glyph{advance: adv.Round()}
		if r != '?' {
			g = f.glyph('?')
		}
		f.cache[r] = g
		return g
	}
	a := image.NewAlpha(image.Rect(0, 0, dr.Dx(), dr.Dy()))
	draw.Draw(a, a.Bounds(), mask, mp, draw.Src)
	g := &glyph{mask: a, off: dr.Min, advance: adv.Round()}
	f.cache[r] = g
	return g
}

func (f *Font) Width(s string) int {
	w := 0
	for _, r := range s {
		w += f.glyph(r).advance
	}
	return w
}

// Text draws s with its top-left at (x,y) and returns the x after it.
func (c *Canvas) Text(f *Font, x, y int, s string, col uint32) int {
	base := y + f.ascent
	cr, cg, cb := chans(col)
	for _, r := range s {
		g := f.glyph(r)
		if g.mask != nil {
			gx, gy := x+g.off.X, base+g.off.Y
			b := g.mask.Bounds()
			for yy := 0; yy < b.Dy(); yy++ {
				py := gy + yy
				if py < 0 || py >= c.H {
					continue
				}
				for xx := 0; xx < b.Dx(); xx++ {
					px := gx + xx
					if px < 0 || px >= c.W {
						continue
					}
					a := int(g.mask.Pix[yy*g.mask.Stride+xx])
					if a == 0 {
						continue
					}
					o := py*c.W + px
					pr, pg, pb := chans(c.Pix[o])
					c.Pix[o] = 0xff000000 | uint32((pr*(255-a)+cr*a)/255)<<16 | uint32((pg*(255-a)+cg*a)/255)<<8 | uint32((pb*(255-a)+cb*a)/255)
				}
			}
		}
		x += g.advance
	}
	return x
}

// TextGlow draws text with a soft phosphor halo.
func (c *Canvas) TextGlow(f *Font, x, y int, s string, col uint32) int {
	halo := mix(colBG, col, 0.3)
	for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		c.Text(f, x+d[0], y+d[1], s, halo)
	}
	return c.Text(f, x, y, s, col)
}

func (c *Canvas) TextCenter(f *Font, cx, y int, s string, col uint32) {
	c.Text(f, cx-f.Width(s)/2, y, s, col)
}

func (c *Canvas) TextRight(f *Font, right, y int, s string, col uint32) {
	c.Text(f, right-f.Width(s), y, s, col)
}

// Button draws a controller button glyph with a caption; returns the x after it.
func (c *Canvas) Button(f *Font, x, y int, btn, caption string, col uint32) int {
	bw := f.Width(btn) + 10
	if bw < f.height+2 {
		bw = f.height + 2
	}
	c.Fill(x, y, bw, f.height+2, col)
	c.Text(f, x+(bw-f.Width(btn))/2, y+1, btn, colBG)
	x += bw + 6
	x = c.Text(f, x, y+1, caption, colPaperDim)
	return x + 14
}

// Chip draws a small outlined or filled tag; returns the x after it.
func (c *Canvas) Chip(f *Font, x, y int, s string, col uint32, filled bool) int {
	w := f.Width(s) + 12
	h := f.height + 4
	if filled {
		c.Fill(x, y, w, h, col)
		c.Text(f, x+6, y+2, s, colBG)
	} else {
		c.Border(x, y, w, h, 1, col)
		c.Text(f, x+6, y+2, s, col)
	}
	return x + w + 6
}

var _ color.Color // keep image/color for ToImage users

func clampf(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }
