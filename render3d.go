package main

// A tiny flat-shaded polygon renderer in the spirit of the Super FX: meshes
// are transformed, back faces culled, faces lit by one directional light and
// fogged into the horizon colour, then painted far-to-near.

import (
	"math"
	"slices"
)

type V3 struct{ X, Y, Z float64 }

func (a V3) Add(b V3) V3      { return V3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a V3) Sub(b V3) V3      { return V3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a V3) Mul(k float64) V3 { return V3{a.X * k, a.Y * k, a.Z * k} }
func (a V3) Dot(b V3) float64 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func (a V3) Len() float64     { return math.Sqrt(a.Dot(a)) }
func (a V3) Cross(b V3) V3    { return V3{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X} }
func (a V3) Norm() V3 {
	l := a.Len()
	if l == 0 {
		return a
	}
	return a.Mul(1 / l)
}
func (a V3) Lerp(b V3, t float64) V3 { return a.Add(b.Sub(a).Mul(t)) }

// M3 is a row-major 3x3 rotation matrix.
type M3 [9]float64

func (m *M3) Apply(v V3) V3 {
	return V3{m[0]*v.X + m[1]*v.Y + m[2]*v.Z, m[3]*v.X + m[4]*v.Y + m[5]*v.Z, m[6]*v.X + m[7]*v.Y + m[8]*v.Z}
}

// Rot builds yaw (Y) * pitch (X) * roll (Z).
func Rot(pitch, yaw, roll float64) M3 {
	cx, sx := math.Cos(pitch), math.Sin(pitch)
	cy, sy := math.Cos(yaw), math.Sin(yaw)
	cz, sz := math.Cos(roll), math.Sin(roll)
	// Ry * Rx * Rz
	return M3{
		cy*cz + sy*sx*sz, -cy*sz + sy*sx*cz, sy * cx,
		cx * sz, cx * cz, -sx,
		-sy*cz + cy*sx*sz, sy*sz + cy*sx*cz, cy * cx,
	}
}

type Face struct {
	Idx []int
	Col uint32
	N   V3
	Two bool // double-sided (thin parts like wings)
}

type Mesh struct {
	V []V3
	F []Face
}

// finish computes face normals; outward=true flips faces so normals point away
// from the mesh centre (for closed convex shapes defined in any winding).
func (m *Mesh) finish(outward bool) *Mesh {
	var c V3
	for _, v := range m.V {
		c = c.Add(v)
	}
	c = c.Mul(1 / float64(len(m.V)))
	for i := range m.F {
		f := &m.F[i]
		a, b, d := m.V[f.Idx[0]], m.V[f.Idx[1]], m.V[f.Idx[2]]
		n := b.Sub(a).Cross(d.Sub(a)).Norm()
		if outward {
			var fc V3
			for _, k := range f.Idx {
				fc = fc.Add(m.V[k])
			}
			fc = fc.Mul(1 / float64(len(f.Idx)))
			if n.Dot(fc.Sub(c)) < 0 {
				n = n.Mul(-1)
				slices.Reverse(f.Idx)
			}
		}
		f.N = n
	}
	return m
}

type poly struct {
	depth float64
	n     int
	x, y  [10]float64
	col   uint32
}

type Renderer struct {
	R        *Canvas
	CamPos   V3
	pc, ps   float64 // camera pitch (looking down) cos/sin
	F        float64 // focal length in pixels
	CX, CY   float64
	Near     float64
	Light    V3
	FogCol   uint32
	FogNear  float64
	FogFar   float64
	polys    []poly
	order    []int32
	vbuf     []V3
	clipA    [12]V3
	clipB    [12]V3
	PolyStat int
}

func NewRenderer(r *Canvas) *Renderer {
	return &Renderer{
		R: r, F: float64(r.W) * 0.66, CX: float64(r.W) / 2, CY: float64(r.H) * 0.5, Near: 0.6, pc: 1,
		Light: V3{-0.45, 0.8, -0.4}.Norm(), FogNear: 70, FogFar: 150,
		polys: make([]poly, 0, 2048), order: make([]int32, 0, 2048), vbuf: make([]V3, 64),
	}
}

func (r *Renderer) Begin() { r.polys = r.polys[:0] }

// SetPitch tilts the camera down by a radians.
func (r *Renderer) SetPitch(a float64) { r.pc, r.ps = math.Cos(a), math.Sin(a) }

// View turns a camera-relative vector into view space (applies the pitch).
func (r *Renderer) View(v V3) V3 {
	return V3{v.X, r.pc*v.Y + r.ps*v.Z, -r.ps*v.Y + r.pc*v.Z}
}

// Horizon is the screen row of the horizon.
func (r *Renderer) Horizon() float64 { return r.CY - r.F*r.ps/r.pc }

func (r *Renderer) fog(col uint32, z float64) uint32 {
	t := (z - r.FogNear) / (r.FogFar - r.FogNear)
	return mix(col, r.FogCol, t)
}

// Project maps a view-space point to the screen.
func (r *Renderer) Project(v V3) (float64, float64) {
	return r.CX + r.F*v.X/v.Z, r.CY - r.F*v.Y/v.Z
}

// ProjectWorld maps a world point; ok=false behind the camera.
func (r *Renderer) ProjectWorld(p V3) (float64, float64, bool) {
	v := r.View(p.Sub(r.CamPos))
	if v.Z < r.Near {
		return 0, 0, false
	}
	x, y := r.Project(v)
	return x, y, true
}

// addPoly clips a view-space polygon against the near plane and queues it.
func (r *Renderer) addPoly(pts []V3, col uint32, depth float64) {
	in := r.clipA[:0]
	allIn := true
	for _, p := range pts {
		if p.Z < r.Near {
			allIn = false
		}
		in = append(in, p)
	}
	if !allIn {
		out := r.clipB[:0]
		for i := range in {
			a, b := in[i], in[(i+1)%len(in)]
			ain, bin := a.Z >= r.Near, b.Z >= r.Near
			if ain {
				out = append(out, a)
			}
			if ain != bin {
				t := (r.Near - a.Z) / (b.Z - a.Z)
				out = append(out, a.Lerp(b, t))
			}
		}
		in = out
		if len(in) < 3 {
			return
		}
	}
	if len(in) > 10 {
		in = in[:10]
	}
	var p poly
	p.n = len(in)
	p.col = col
	p.depth = depth
	minx, maxx, miny, maxy := 1e9, -1e9, 1e9, -1e9
	for i, v := range in {
		p.x[i], p.y[i] = r.Project(v)
		minx, maxx = math.Min(minx, p.x[i]), math.Max(maxx, p.x[i])
		miny, maxy = math.Min(miny, p.y[i]), math.Max(maxy, p.y[i])
	}
	if maxx < 0 || maxy < 0 || minx >= float64(r.R.W) || miny >= float64(r.R.H) {
		return
	}
	r.polys = append(r.polys, p)
}

// Mesh queues a mesh at pos with rotation rot and uniform scale s.
// flash (0..1) whitens it; fade (0..1) dims it toward the fog colour.
func (r *Renderer) Mesh(m *Mesh, pos V3, rot *M3, s float64, flash float64) {
	if len(r.vbuf) < len(m.V) {
		r.vbuf = make([]V3, len(m.V))
	}
	vp := pos.Sub(r.CamPos)
	if vp.Z < -8 || vp.Z > r.FogFar+10 {
		return
	}
	for i, v := range m.V {
		r.vbuf[i] = r.View(rot.Apply(v.Mul(s)).Add(vp))
	}
	var pts [8]V3
	for _, f := range m.F {
		var c V3
		for k, idx := range f.Idx {
			pts[k] = r.vbuf[idx]
			c = c.Add(pts[k])
		}
		c = c.Mul(1 / float64(len(f.Idx)))
		n := rot.Apply(f.N)
		if r.View(n).Dot(c) >= 0 {
			if !f.Two {
				continue
			}
			n = n.Mul(-1)
		}
		lum := 0.42 + 0.68*math.Max(0, n.Dot(r.Light))
		col := scale(f.Col, lum)
		if flash > 0 {
			col = mix(col, colWhite, flash)
		}
		col = r.fog(col, c.Z)
		r.addPoly(pts[:len(f.Idx)], col, c.Len())
	}
}

// Billboard queues a flat octagon facing the camera (sparks, glows).
func (r *Renderer) Billboard(pos V3, rad float64, col uint32, fogged bool) {
	v := r.View(pos.Sub(r.CamPos))
	if v.Z < r.Near+0.1 {
		return
	}
	if fogged {
		col = r.fog(col, v.Z)
	}
	var pts [8]V3
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		pts[i] = V3{v.X + math.Cos(a)*rad, v.Y + math.Sin(a)*rad, v.Z}
	}
	r.addPoly(pts[:], col, v.Len()-rad*0.5)
}

// Beam queues a camera-facing quad from a to b with world width w.
func (r *Renderer) Beam(a, b V3, w float64, col uint32) {
	va, vb := r.View(a.Sub(r.CamPos)), r.View(b.Sub(r.CamPos))
	if va.Z < r.Near && vb.Z < r.Near {
		return
	}
	d := vb.Sub(va)
	side := d.Cross(va.Lerp(vb, 0.5)).Norm().Mul(w / 2)
	if side.Len() == 0 {
		side = V3{w / 2, 0, 0}
	}
	pts := [4]V3{va.Add(side), vb.Add(side), vb.Sub(side), va.Sub(side)}
	mid := va.Lerp(vb, 0.5)
	r.addPoly(pts[:], r.fog(col, mid.Z), mid.Len())
}

// Flush paints everything queued, far to near.
func (r *Renderer) Flush() {
	r.order = r.order[:0]
	for i := range r.polys {
		r.order = append(r.order, int32(i))
	}
	ps := r.polys
	slices.SortFunc(r.order, func(a, b int32) int {
		da, db := ps[a].depth, ps[b].depth
		if da > db {
			return -1
		}
		if da < db {
			return 1
		}
		return 0
	})
	for _, i := range r.order {
		p := &r.polys[i]
		for k := 1; k+1 < p.n; k++ {
			r.tri(p.x[0], p.y[0], p.x[k], p.y[k], p.x[k+1], p.y[k+1], p.col)
		}
	}
	r.PolyStat = len(r.polys)
}

// tri fills a triangle using pixel-centre sampling (no gaps or overlaps
// between triangles that share an edge).
func (r *Renderer) tri(x0, y0, x1, y1, x2, y2 float64, col uint32) {
	if y1 < y0 {
		x0, y0, x1, y1 = x1, y1, x0, y0
	}
	if y2 < y0 {
		x0, y0, x2, y2 = x2, y2, x0, y0
	}
	if y2 < y1 {
		x1, y1, x2, y2 = x2, y2, x1, y1
	}
	if y2-y0 < 1e-9 {
		return
	}
	c := r.R
	ys := int(math.Ceil(y0 - 0.5))
	ye := int(math.Ceil(y2 - 0.5))
	if ys < 0 {
		ys = 0
	}
	if ye > c.H {
		ye = c.H
	}
	inv02 := (x2 - x0) / (y2 - y0)
	var inv01, inv12 float64
	if y1 > y0 {
		inv01 = (x1 - x0) / (y1 - y0)
	}
	if y2 > y1 {
		inv12 = (x2 - x1) / (y2 - y1)
	}
	for y := ys; y < ye; y++ {
		py := float64(y) + 0.5
		xa := x0 + inv02*(py-y0)
		var xb float64
		if py < y1 {
			xb = x0 + inv01*(py-y0)
		} else {
			xb = x1 + inv12*(py-y1)
		}
		if xa > xb {
			xa, xb = xb, xa
		}
		xs := int(math.Ceil(xa - 0.5))
		xe := int(math.Ceil(xb - 0.5))
		if xs < 0 {
			xs = 0
		}
		if xe > c.W {
			xe = c.W
		}
		if xe <= xs {
			continue
		}
		row := c.Pix[y*c.W+xs : y*c.W+xe]
		for i := range row {
			row[i] = col
		}
	}
}
