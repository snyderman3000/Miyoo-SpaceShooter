package main

import (
	"math"
	"math/rand"
)

// Ship: an original delta-wing fighter, nose toward +Z.
func shipMesh() *Mesh {
	v := []V3{
		{0, 0, 2.1},           // 0 nose
		{0, 0.44, 0.0},        // 1 canopy top
		{-0.52, 0.05, -0.35},  // 2 left shoulder
		{0.52, 0.05, -0.35},   // 3 right shoulder
		{0, -0.32, -0.2},      // 4 belly
		{0, 0.12, -1.15},      // 5 tail
		{-2.05, -0.28, -1.05}, // 6 left wing tip
		{2.05, -0.28, -1.05},  // 7 right wing tip
		{-1.8, 0.78, -1.35},   // 8 left fin top
		{1.8, 0.78, -1.35},    // 9 right fin top
		{-0.42, -0.02, -1.05}, // 10 left wing root (back)
		{0.42, -0.02, -1.05},  // 11 right wing root (back)
		{0, 0.24, 0.85},       // 12 canopy front
		{-1.62, -0.22, -0.62}, // 13 left fin root
		{1.62, -0.22, -0.62},  // 14 right fin root
		{-0.38, 0.0, 0.95},    // 15 left wing root (front)
		{0.38, 0.0, 0.95},     // 16 right wing root (front)
	}
	v[6], v[7] = V3{-2.1, -0.2, -1.0}, V3{2.1, -0.2, -1.0}
	hull := colPaper
	hullDk := rgb(0x9a, 0x96, 0x8a)
	f := []Face{
		{Idx: []int{0, 12, 2}, Col: hull},
		{Idx: []int{0, 12, 3}, Col: hull},
		{Idx: []int{12, 1, 2}, Col: colCyan},
		{Idx: []int{12, 1, 3}, Col: colCyan},
		{Idx: []int{1, 2, 5}, Col: hullDk},
		{Idx: []int{1, 3, 5}, Col: hullDk},
		{Idx: []int{0, 2, 4}, Col: rgb(0x6c, 0x6a, 0x64)},
		{Idx: []int{0, 3, 4}, Col: rgb(0x6c, 0x6a, 0x64)},
		{Idx: []int{2, 4, 5}, Col: rgb(0x48, 0x46, 0x42)},
		{Idx: []int{3, 4, 5}, Col: rgb(0x48, 0x46, 0x42)},
		{Idx: []int{15, 6, 10}, Col: colAmber},
		{Idx: []int{16, 7, 11}, Col: colAmber},
		{Idx: []int{2, 10, 5}, Col: colAmberDim},
		{Idx: []int{3, 11, 5}, Col: colAmberDim},
		{Idx: []int{6, 8, 13}, Col: colMagenta},
		{Idx: []int{7, 9, 14}, Col: colMagenta},
	}
	for i := range f {
		f[i].Two = true
	}
	return (&Mesh{V: v, F: f}).finish(false)
}

func icosahedron() []V3 {
	p := (1 + math.Sqrt(5)) / 2
	v := []V3{
		{-1, p, 0}, {1, p, 0}, {-1, -p, 0}, {1, -p, 0},
		{0, -1, p}, {0, 1, p}, {0, -1, -p}, {0, 1, -p},
		{p, 0, -1}, {p, 0, 1}, {-p, 0, -1}, {-p, 0, 1},
	}
	for i := range v {
		v[i] = v[i].Norm()
	}
	return v
}

var icoFaces = [][]int{
	{0, 11, 5}, {0, 5, 1}, {0, 1, 7}, {0, 7, 10}, {0, 10, 11},
	{1, 5, 9}, {5, 11, 4}, {11, 10, 2}, {10, 7, 6}, {7, 1, 8},
	{3, 9, 4}, {3, 4, 2}, {3, 2, 6}, {3, 6, 8}, {3, 8, 9},
	{4, 9, 5}, {2, 4, 11}, {6, 2, 10}, {8, 6, 7}, {9, 8, 1},
}

// asteroidMesh: a lumpy icosahedron in dusty violet with magenta veins.
func asteroidMesh(rng *rand.Rand) *Mesh {
	v := icosahedron()
	for i := range v {
		v[i] = v[i].Mul(0.72 + rng.Float64()*0.45)
	}
	base := []uint32{rgb(0x6e, 0x60, 0x7c), rgb(0x5c, 0x50, 0x6a), rgb(0x7a, 0x6c, 0x84), rgb(0x52, 0x46, 0x5e)}
	m := &Mesh{V: v}
	for i, fi := range icoFaces {
		col := base[rng.Intn(len(base))]
		if i%7 == 3 {
			col = rgb(0xa8, 0x2a, 0x68)
		}
		m.F = append(m.F, Face{Idx: append([]int(nil), fi...), Col: col})
	}
	return m.finish(true)
}

// boxMesh: an axis-aligned box with half extents h; each face coloured by fn.
func boxMesh(h V3, side, front, top uint32) *Mesh {
	v := []V3{
		{-h.X, -h.Y, -h.Z}, {h.X, -h.Y, -h.Z}, {h.X, h.Y, -h.Z}, {-h.X, h.Y, -h.Z},
		{-h.X, -h.Y, h.Z}, {h.X, -h.Y, h.Z}, {h.X, h.Y, h.Z}, {-h.X, h.Y, h.Z},
	}
	f := []Face{
		{Idx: []int{0, 1, 2, 3}, Col: front}, // -Z faces the player
		{Idx: []int{4, 5, 6, 7}, Col: front},
		{Idx: []int{0, 3, 7, 4}, Col: side},
		{Idx: []int{1, 2, 6, 5}, Col: side},
		{Idx: []int{3, 2, 6, 7}, Col: top},
		{Idx: []int{0, 1, 5, 4}, Col: top},
	}
	return (&Mesh{V: v, F: f}).finish(true)
}

// droneMesh: the target — a stretched octahedron with a ring of fins.
func droneMesh(a, b uint32) *Mesh {
	v := []V3{{0, 1.15, 0}, {0, -1.15, 0}, {0.85, 0, 0}, {-0.85, 0, 0}, {0, 0, 0.85}, {0, 0, -0.85}}
	m := &Mesh{V: v}
	tris := [][]int{{0, 2, 4}, {0, 4, 3}, {0, 3, 5}, {0, 5, 2}, {1, 4, 2}, {1, 3, 4}, {1, 5, 3}, {1, 2, 5}}
	for i, t := range tris {
		col := a
		if i%2 == 1 {
			col = b
		}
		m.F = append(m.F, Face{Idx: t, Col: col})
	}
	return m.finish(true)
}

// cubeMesh: the gold bonus target.
func cubeMesh() *Mesh {
	m := boxMesh(V3{0.62, 0.62, 0.62}, colAmber, rgb(0xff, 0xd8, 0x6a), colAmberDim)
	return m
}

// shardMesh: a single debris triangle (double-sided).
func shardMesh(col uint32) *Mesh {
	return (&Mesh{V: []V3{{0, 0.5, 0}, {-0.4, -0.3, 0.1}, {0.45, -0.25, -0.1}},
		F: []Face{{Idx: []int{0, 1, 2}, Col: col, Two: true}}}).finish(false)
}

type Models struct {
	Ship      *Mesh
	Asteroids []*Mesh
	Pylon     *Mesh
	Beam      *Mesh
	Drone     *Mesh
	Bonus     *Mesh
	Shards    map[uint32]*Mesh
}

const (
	pylonHalfW = 0.65
	beamHalfH  = 0.5
)

func buildModels() *Models {
	rng := rand.New(rand.NewSource(7))
	m := &Models{Ship: shipMesh(), Shards: map[uint32]*Mesh{}}
	for i := 0; i < 6; i++ {
		m.Asteroids = append(m.Asteroids, asteroidMesh(rng))
	}
	m.Pylon = boxMesh(V3{pylonHalfW, 14, pylonHalfW}, rgb(0x8c, 0x1c, 0x54), colMagenta, colAmber)
	m.Beam = boxMesh(V3{16, beamHalfH, pylonHalfW}, rgb(0x8c, 0x1c, 0x54), colMagenta, rgb(0xb8, 0x2a, 0x70))
	m.Drone = droneMesh(colCyan, rgb(0x16, 0x8a, 0x80))
	m.Bonus = cubeMesh()
	return m
}

func (m *Models) Shard(col uint32) *Mesh {
	if s, ok := m.Shards[col]; ok {
		return s
	}
	s := shardMesh(col)
	m.Shards[col] = s
	return s
}
