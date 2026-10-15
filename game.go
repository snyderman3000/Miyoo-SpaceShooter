package main

import (
	"fmt"
	"math"
	"math/rand"
)

type Kind int

const (
	KAsteroid Kind = iota
	KPylon
	KBeam
	KDrone
	KBonus
)

func (k Kind) Target() bool { return k == KDrone || k == KBonus }

type Obj struct {
	Kind      Kind
	Pos       V3
	Ang, Spin V3
	R         float64 // collision radius (spheres)
	Half      V3      // half extents (boxes)
	Mesh      *Mesh
	S         float64 // render scale
	BaseX     float64
	BaseY     float64
	Amp, Freq float64
	Phase     float64
	Axis      int // 0 x, 1 y, 2 circle
	Age       float64
	Flash     float64
	Dead      bool
}

type Bolt struct {
	Pos, Prev V3
	Dead      bool
}

type Part struct {
	Pos, Vel  V3
	Ang, Spin V3
	Life, Max float64
	Mesh      *Mesh
	Col       uint32
	Size      float64
}

type Ring struct {
	Pos    V3
	T, Max float64
	Size   float64
	Col    uint32
}

type Popup struct {
	Pos  V3
	T    float64
	Text string
	Col  uint32
}

type State int

const (
	StTitle State = iota
	StCountdown
	StPlay
	StDying
	StFinish
	StResults
)

type Input struct {
	Up, Down, Left, Right, Fire, BankL, BankR bool
}

type Result struct {
	Survived  bool
	Score     int
	Bonus     int
	Hits      int
	Spawned   int
	Shots     int
	Time      float64
	BestChain int
	Rank      int // place in the top-score table, -1 if not placed
	Grade     string
	Killer    string
}

const (
	roundTime   = 60.0
	spawnZ      = 145.0
	boltSpeed   = 190.0
	fireDelay   = 0.13
	areaX       = 5.6
	areaY       = 3.6
	survivalPts = 1000
	shipScale   = 0.9
)

type Game struct {
	M      *Models
	rng    *rand.Rand
	Audio  *Audio
	Scores *ScoreBook
	Cfg    *Config

	State  State
	T      float64 // time in the current state
	Paused bool
	Clock  float64 // total running time (animation)

	TimeLeft float64
	Speed    float64
	Dist     float64

	Ship      V3
	SVel      V3
	Bank      float64
	Pitch     float64
	Yaw       float64
	ShipAlive bool
	In        Input

	Objs  []*Obj
	Bolts []Bolt
	Parts []Part
	Rings []Ring
	Pops  []Popup

	Score, Hits, Shots, Spawned int
	Chain, BestChain, Mult      int
	ChainT                      float64

	nextObs, nextTgt, fireCD float64
	lastTick                 int
	FlashT                   float64
	Shake                    float64

	Res  Result
	Quit bool
	God  bool // tests: no collisions
}

func NewGame(m *Models, seed int64, audio *Audio, scores *ScoreBook, cfg *Config) *Game {
	g := &Game{M: m, rng: rand.New(rand.NewSource(seed)), Audio: audio, Scores: scores, Cfg: cfg}
	g.State = StTitle
	g.Speed = 22
	g.Mult = 1
	g.Ship = V3{0, -0.4, 0}
	g.ShipAlive = true
	g.Audio.Music(MusicTitle)
	return g
}

func (g *Game) elapsed() float64 { return roundTime - g.TimeLeft }

// Start begins a new round (countdown first).
func (g *Game) Start() {
	g.Objs = g.Objs[:0]
	g.Bolts = g.Bolts[:0]
	g.Parts = g.Parts[:0]
	g.Rings = g.Rings[:0]
	g.Pops = g.Pops[:0]
	g.Score, g.Hits, g.Shots, g.Spawned = 0, 0, 0, 0
	g.Chain, g.BestChain, g.Mult, g.ChainT = 0, 0, 1, 0
	g.TimeLeft = roundTime
	g.Speed = 34
	g.Ship = V3{0, -0.4, 0}
	g.SVel = V3{}
	g.Bank, g.Pitch, g.Yaw = 0, 0, 0
	g.ShipAlive = true
	g.nextObs = 3.4 // the first obstacles arrive a little after GO
	g.nextTgt = 0.6
	g.fireCD = 0
	g.lastTick = 11
	g.FlashT, g.Shake = 0, 0
	g.Paused = false
	g.setState(StCountdown)
	g.Audio.Music(MusicNone)
	g.Audio.Play(SfxBeep)
}

func (g *Game) setState(s State) {
	g.State = s
	g.T = 0
}

// Press handles a single button press (menus, pause, state changes).
func (g *Game) Press(b int) {
	switch g.State {
	case StTitle:
		switch b {
		case BtnA, BtnStart:
			g.Start()
		case BtnSelect:
			g.toggleMusic()
		case BtnB, BtnMenu:
			g.Quit = true
		}
	case StCountdown, StPlay:
		if g.Paused {
			switch b {
			case BtnA, BtnStart:
				g.Paused = false
				g.Audio.Pause(false)
			case BtnSelect:
				g.toggleMusic()
			case BtnB:
				g.Paused = false
				g.Audio.Pause(false)
				g.toTitle()
			case BtnMenu:
				g.Quit = true
			}
			return
		}
		switch b {
		case BtnStart, BtnMenu:
			g.Paused = true
			g.Audio.Pause(true)
		case BtnSelect:
			g.toggleMusic()
		}
	case StResults:
		if g.T < 0.8 {
			return // don't skip the results by mashing fire
		}
		switch b {
		case BtnA, BtnStart:
			g.Start()
		case BtnB:
			g.toTitle()
		case BtnMenu:
			g.Quit = true
		case BtnSelect:
			g.toggleMusic()
		}
	}
}

func (g *Game) toTitle() {
	g.Objs = g.Objs[:0]
	g.Bolts = g.Bolts[:0]
	g.Parts = g.Parts[:0]
	g.Rings = g.Rings[:0]
	g.Pops = g.Pops[:0]
	g.Speed = 22
	g.Ship = V3{0, -0.4, 0}
	g.ShipAlive = true
	g.setState(StTitle)
	g.Audio.Music(MusicTitle)
}

func (g *Game) toggleMusic() {
	g.Cfg.Music = !g.Cfg.Music
	g.Cfg.Save()
	g.Audio.SetMusicOn(g.Cfg.Music)
}

// Update advances the simulation by dt seconds.
func (g *Game) Update(dt float64) {
	if g.Paused {
		return
	}
	g.T += dt
	g.Clock += dt
	g.Dist += g.Speed * dt
	if g.FlashT > 0 {
		g.FlashT -= dt
	}
	if g.Shake > 0 {
		g.Shake = math.Max(0, g.Shake-dt*1.6)
	}

	switch g.State {
	case StTitle:
		g.Speed = 22
	case StCountdown:
		n := int(g.T)
		prev := int(g.T - dt)
		if n != prev && n >= 1 && n <= 2 {
			g.Audio.Play(SfxBeep)
		}
		if g.T >= 3 {
			g.setState(StPlay)
			g.Audio.Play(SfxGo)
			g.Audio.Music(MusicPlay)
		}
		g.moveShip(dt)
		g.spawn(dt, false)
	case StPlay:
		g.TimeLeft -= dt
		if s := int(math.Ceil(g.TimeLeft)); s < g.lastTick && s <= 10 && s > 0 {
			g.lastTick = s
			g.Audio.Play(SfxTick)
		}
		g.Speed = 34 + 22*g.elapsed()/roundTime
		g.moveShip(dt)
		g.fire(dt)
		g.spawn(dt, true)
		if g.TimeLeft <= 0 {
			g.TimeLeft = 0
			g.finish(true)
		}
	case StDying:
		g.Speed = math.Max(6, g.Speed*(1-dt*1.2))
		if g.T > 2.3 {
			g.toResults()
		}
	case StFinish:
		g.Ship.Z += dt * (8 + g.T*70)
		g.Ship.Y += dt * g.T * 2
		g.Pitch += (-0.25 - g.Pitch) * math.Min(1, dt*4)
		g.Bank += (0 - g.Bank) * math.Min(1, dt*4)
		if g.T > 1.9 {
			g.toResults()
		}
	case StResults:
		g.Speed = 18
	}

	g.updateWorld(dt)
	if g.State == StPlay {
		g.collide()
	}
	if g.ChainT > 0 {
		g.ChainT -= dt
		if g.ChainT <= 0 {
			g.Chain = 0
			g.Mult = 1
		}
	}
}

func (g *Game) moveShip(dt float64) {
	ax, ay := 0.0, 0.0
	if g.In.Left {
		ax--
	}
	if g.In.Right {
		ax++
	}
	if g.In.Up {
		ay++
	}
	if g.In.Down {
		ay--
	}
	if g.Cfg.InvertY {
		ay = -ay
	}
	maxX, maxY := 9.5, 7.5
	bank := 0.0
	if g.In.BankL && !g.In.BankR {
		bank = 1
	} else if g.In.BankR && !g.In.BankL {
		bank = -1
	}
	if bank != 0 {
		maxX = 14 // knife-edge: tighter, faster turns
		if ax == 0 {
			ax = -bank
		}
	}
	k := math.Min(1, dt*7)
	g.SVel.X += (ax*maxX - g.SVel.X) * k
	g.SVel.Y += (ay*maxY - g.SVel.Y) * k
	g.Ship.X += g.SVel.X * dt
	g.Ship.Y += g.SVel.Y * dt
	if math.Abs(g.Ship.X) > areaX {
		g.Ship.X = math.Copysign(areaX, g.Ship.X)
		g.SVel.X = 0
	}
	if math.Abs(g.Ship.Y) > areaY {
		g.Ship.Y = math.Copysign(areaY, g.Ship.Y)
		g.SVel.Y = 0
	}
	tb := -g.SVel.X / 9.5 * 0.55
	if bank != 0 {
		tb = bank * 1.4
	}
	g.Bank += (tb - g.Bank) * math.Min(1, dt*9)
	g.Pitch += (-g.SVel.Y/maxY*0.3 - g.Pitch) * math.Min(1, dt*8)
	g.Yaw += (g.SVel.X/9.5*0.14 - g.Yaw) * math.Min(1, dt*8)
}

func (g *Game) fire(dt float64) {
	g.fireCD -= dt
	if !g.In.Fire || g.fireCD > 0 || !g.ShipAlive {
		return
	}
	g.fireCD = fireDelay
	g.Shots++
	for _, sx := range []float64{-0.62, 0.62} {
		p := V3{g.Ship.X + sx*math.Cos(g.Bank), g.Ship.Y - 0.05 + sx*math.Sin(g.Bank), g.Ship.Z + 1.3}
		g.Bolts = append(g.Bolts, Bolt{Pos: p, Prev: p})
	}
	g.Audio.Play(SfxLaser)
}

func (g *Game) rnd(a, b float64) float64 { return a + g.rng.Float64()*(b-a) }

func (g *Game) spawn(dt float64, obstacles bool) {
	e := g.elapsed()
	p := math.Min(1, e/roundTime)
	g.nextTgt -= dt
	if g.nextTgt <= 0 {
		g.nextTgt = (0.72 - 0.24*p) * g.rnd(0.75, 1.25)
		g.spawnTarget(e)
	}
	if !obstacles {
		return
	}
	g.nextObs -= dt
	if g.nextObs <= 0 {
		g.nextObs = (1.05 - 0.6*p) * g.rnd(0.75, 1.25)
		g.spawnObstacle(e)
	}
}

func (g *Game) addAsteroid(x, y, size float64) {
	m := g.M.Asteroids[g.rng.Intn(len(g.M.Asteroids))]
	g.Objs = append(g.Objs, &Obj{Kind: KAsteroid, Pos: V3{x, y, spawnZ + g.rnd(-3, 3)}, Mesh: m, S: size, R: size * 0.82,
		Ang:  V3{g.rnd(0, 6), g.rnd(0, 6), g.rnd(0, 6)},
		Spin: V3{g.rnd(-1.2, 1.2), g.rnd(-1.2, 1.2), g.rnd(-0.8, 0.8)}})
}

func (g *Game) spawnObstacle(e float64) {
	type opt struct {
		w   float64
		min float64
		f   func()
	}
	opts := []opt{
		{5, 0, func() { g.addAsteroid(g.rnd(-6.5, 6.5), g.rnd(-4, 4), g.rnd(1.0, 2.3)) }},
		{2.5, 14, func() { // aimed at where the player is now
			g.addAsteroid(clampf(g.Ship.X+g.rnd(-1, 1), -6, 6), clampf(g.Ship.Y+g.rnd(-0.8, 0.8), -4, 4), g.rnd(1.3, 2.0))
		}},
		{2, 10, func() { // cluster
			cx, cy := g.rnd(-4, 4), g.rnd(-2.5, 2.5)
			for i := 0; i < 3; i++ {
				g.addAsteroid(cx+g.rnd(-2.6, 2.6), cy+g.rnd(-2, 2), g.rnd(0.8, 1.4))
			}
		}},
		{2, 5, func() {
			h := V3{pylonHalfW, 14, pylonHalfW}
			g.Objs = append(g.Objs, &Obj{Kind: KPylon, Pos: V3{g.rnd(-4.5, 4.5), 0, spawnZ}, Mesh: g.M.Pylon, S: 1, Half: h})
		}},
		{1.8, 8, func() {
			h := V3{16, beamHalfH, pylonHalfW}
			g.Objs = append(g.Objs, &Obj{Kind: KBeam, Pos: V3{0, g.rnd(-2.4, 2.4), spawnZ}, Mesh: g.M.Beam, S: 1, Half: h})
		}},
	}
	total := 0.0
	for _, o := range opts {
		if e >= o.min {
			total += o.w
		}
	}
	r := g.rng.Float64() * total
	for _, o := range opts {
		if e < o.min {
			continue
		}
		if r < o.w {
			o.f()
			return
		}
		r -= o.w
	}
}

// clearAt reports whether a target at (x,y) near the spawn plane would sit
// inside or right next to an obstacle.
func (g *Game) clearAt(x, y float64) bool {
	p := V3{x, y, spawnZ}
	for _, o := range g.Objs {
		if o.Kind.Target() || math.Abs(o.Pos.Z-spawnZ) > 9 {
			continue
		}
		switch o.Kind {
		case KAsteroid:
			if p.Sub(o.Pos).Len() < o.R+2.0 {
				return false
			}
		case KPylon:
			if math.Abs(x-o.Pos.X) < 2.2 {
				return false
			}
		case KBeam:
			if math.Abs(y-o.Pos.Y) < 1.9 {
				return false
			}
		}
	}
	return true
}

func (g *Game) spawnTarget(e float64) {
	var x, y float64
	ok := false
	for i := 0; i < 8 && !ok; i++ {
		x, y = g.rnd(-5, 5), g.rnd(-3.2, 3.2)
		ok = g.clearAt(x, y)
	}
	if !ok {
		return
	}
	o := &Obj{Kind: KDrone, Pos: V3{x, y, spawnZ}, Mesh: g.M.Drone, S: 0.95, R: 1.3, BaseX: x, BaseY: y,
		Spin: V3{0, g.rnd(1.5, 2.5), 0}, Phase: g.rnd(0, 6.3)}
	if e > 4 && g.rng.Float64() < 0.08 {
		o.Kind, o.Mesh, o.S, o.R = KBonus, g.M.Bonus, 1, 1.1
		o.Axis, o.Amp, o.Freq = 2, 1.8, 2.2
		o.Spin = V3{2.4, 3.1, 1.3}
		o.BaseX, o.BaseY = clampf(x, -3.4, 3.4), clampf(y, -1.6, 1.6)
	} else if g.rng.Float64() < 0.25+0.35*e/roundTime {
		o.Axis = g.rng.Intn(2)
		o.Amp = g.rnd(1.2, 2.8)
		o.Freq = g.rnd(0.9, 1.7)
	}
	g.Spawned++
	g.Objs = append(g.Objs, o)
}

func (g *Game) updateWorld(dt float64) {
	// objects
	live := g.Objs[:0]
	for _, o := range g.Objs {
		o.Age += dt
		o.Pos.Z -= g.Speed * dt
		o.Ang = o.Ang.Add(o.Spin.Mul(dt))
		if o.Flash > 0 {
			o.Flash -= dt * 4
		}
		if o.Amp > 0 {
			ph := o.Phase + o.Age*o.Freq
			switch o.Axis {
			case 0:
				o.Pos.X = clampf(o.BaseX+o.Amp*math.Sin(ph), -6, 6)
			case 1:
				o.Pos.Y = clampf(o.BaseY+o.Amp*math.Sin(ph), -3.8, 3.8)
			case 2:
				o.Pos.X = o.BaseX + o.Amp*math.Cos(ph)
				o.Pos.Y = o.BaseY + o.Amp*math.Sin(ph)
			}
		}
		if !o.Dead && o.Pos.Z > -16 {
			live = append(live, o)
		}
	}
	for i := len(live); i < len(g.Objs); i++ {
		g.Objs[i] = nil
	}
	g.Objs = live

	// bolts
	step := boltSpeed * dt
	bl := g.Bolts[:0]
	for _, b := range g.Bolts {
		b.Prev = b.Pos
		b.Pos.Z += step
		if g.State == StPlay || g.State == StDying || g.State == StFinish {
			g.boltHits(&b, dt)
		}
		if !b.Dead && b.Pos.Z < spawnZ+5 {
			bl = append(bl, b)
		}
	}
	g.Bolts = bl

	// particles
	pl := g.Parts[:0]
	for _, p := range g.Parts {
		p.Life -= dt
		p.Pos = p.Pos.Add(p.Vel.Mul(dt))
		p.Pos.Z -= g.Speed * dt * 0.5
		p.Vel = p.Vel.Mul(1 - math.Min(0.9, dt*1.1))
		p.Ang = p.Ang.Add(p.Spin.Mul(dt))
		if p.Life > 0 {
			pl = append(pl, p)
		}
	}
	g.Parts = pl

	rl := g.Rings[:0]
	for _, r := range g.Rings {
		r.T += dt
		r.Pos.Z -= g.Speed * dt * 0.5
		if r.T < r.Max {
			rl = append(rl, r)
		}
	}
	g.Rings = rl

	pp := g.Pops[:0]
	for _, p := range g.Pops {
		p.T += dt
		p.Pos.Z -= g.Speed * dt * 0.3
		if p.T < 0.9 {
			pp = append(pp, p)
		}
	}
	g.Pops = pp
}

// segSphere: does the segment a→b pass within r of the origin?
func segSphere(a, b V3, r float64) bool {
	d := b.Sub(a)
	l2 := d.Dot(d)
	t := 0.0
	if l2 > 0 {
		t = clampf(-a.Dot(d)/l2, 0, 1)
	}
	return a.Add(d.Mul(t)).Len() < r
}

func inBox(p, c, h V3, pad float64) bool {
	return math.Abs(p.X-c.X) < h.X+pad && math.Abs(p.Y-c.Y) < h.Y+pad && math.Abs(p.Z-c.Z) < h.Z+pad
}

func (g *Game) boltHits(b *Bolt, dt float64) {
	back := V3{0, 0, g.Speed * dt}
	for _, o := range g.Objs {
		if o.Dead {
			continue
		}
		prev := o.Pos.Add(back)
		switch o.Kind {
		case KDrone, KBonus, KAsteroid:
			r := o.R
			if segSphere(b.Prev.Sub(prev), b.Pos.Sub(o.Pos), r) {
				b.Dead = true
				if o.Kind.Target() {
					g.kill(o)
				} else {
					o.Flash = 0.6
					g.spark(b.Pos, colPaper, 4)
					g.Audio.Play(SfxClink)
				}
				return
			}
		case KPylon, KBeam:
			for s := 0.0; s <= 1.0; s += 0.2 {
				p := b.Prev.Sub(prev).Lerp(b.Pos.Sub(o.Pos), s)
				if inBox(p, V3{}, o.Half, 0.1) {
					b.Dead = true
					o.Flash = 0.5
					g.spark(o.Pos.Add(p), colAmber, 4)
					g.Audio.Play(SfxClink)
					return
				}
			}
		}
	}
}

func (g *Game) kill(o *Obj) {
	o.Dead = true
	g.Hits++
	if g.ChainT > 0 {
		g.Chain++
	} else {
		g.Chain = 1
	}
	g.ChainT = 1.25
	if g.Chain > g.BestChain {
		g.BestChain = g.Chain
	}
	g.Mult = 1 + min(3, (g.Chain-1)/3)
	pts := 100
	col := colCyan
	if o.Kind == KBonus {
		pts = 300
		col = colAmber
		g.Audio.Play(SfxBonus)
	} else {
		g.Audio.Play(SfxHit)
	}
	pts *= g.Mult
	g.Score += pts
	g.Pops = append(g.Pops, Popup{Pos: o.Pos, Text: fmt.Sprintf("+%d", pts), Col: col})
	g.explode(o.Pos, col, rgb(0x16, 0x8a, 0x80), 9, 1)
}

func (g *Game) spark(p V3, col uint32, n int) {
	for i := 0; i < n; i++ {
		v := V3{g.rnd(-1, 1), g.rnd(-1, 1), g.rnd(-1, 0.3)}.Norm().Mul(g.rnd(4, 9))
		g.Parts = append(g.Parts, Part{Pos: p, Vel: v, Life: 0.3, Max: 0.3, Col: col, Size: 0.12})
	}
}

func (g *Game) explode(p V3, a, b uint32, n int, size float64) {
	for i := 0; i < n; i++ {
		col := a
		if i%2 == 1 {
			col = b
		}
		v := V3{g.rnd(-1, 1), g.rnd(-1, 1), g.rnd(-1, 1)}.Norm().Mul(g.rnd(5, 13) * size)
		life := g.rnd(0.7, 1.3)
		g.Parts = append(g.Parts, Part{Pos: p, Vel: v, Life: life, Max: life, Mesh: g.M.Shard(col),
			Ang: V3{g.rnd(0, 6), g.rnd(0, 6), 0}, Spin: V3{g.rnd(-9, 9), g.rnd(-9, 9), g.rnd(-9, 9)}, Size: size * g.rnd(0.6, 1.1)})
	}
	for i := 0; i < n; i++ {
		v := V3{g.rnd(-1, 1), g.rnd(-1, 1), g.rnd(-1, 1)}.Norm().Mul(g.rnd(6, 16) * size)
		g.Parts = append(g.Parts, Part{Pos: p, Vel: v, Life: 0.45, Max: 0.45, Col: colPaper, Size: 0.16 * size})
	}
	g.Rings = append(g.Rings, Ring{Pos: p, Max: 0.45, Size: 1.6 * size, Col: a})
}

func (g *Game) collide() {
	if !g.ShipAlive || g.God {
		return
	}
	// a slim ellipsoid hitbox: body plus two wing points (forgiving, Starfox-style)
	cb, sb := math.Cos(g.Bank), math.Sin(g.Bank)
	wing := 1.0 * shipScale / 0.75
	pts := [3]struct {
		p V3
		r float64
	}{
		{g.Ship, 0.55},
		{g.Ship.Add(V3{-wing * cb, -wing * sb, -0.4}), 0.32},
		{g.Ship.Add(V3{wing * cb, wing * sb, -0.4}), 0.32},
	}
	for _, o := range g.Objs {
		if o.Dead || o.Kind.Target() || math.Abs(o.Pos.Z-g.Ship.Z) > 16 {
			continue
		}
		for _, q := range pts {
			hit := false
			switch o.Kind {
			case KAsteroid:
				hit = q.p.Sub(o.Pos).Len() < o.R+q.r
			case KPylon, KBeam:
				hit = inBox(q.p, o.Pos, o.Half, q.r)
			}
			if hit {
				g.die(o)
				return
			}
		}
	}
}

func (g *Game) die(o *Obj) {
	g.ShipAlive = false
	o.Flash = 1
	g.explode(g.Ship, colAmber, colPaper, 16, 0.8)
	g.explode(g.Ship, colMagenta, colAmberDim, 10, 0.6)
	g.Rings = append(g.Rings, Ring{Pos: g.Ship, Max: 0.8, Size: 2.2, Col: colPaper})
	g.FlashT = 0.35
	g.Shake = 1
	g.Bolts = g.Bolts[:0]
	g.Audio.Music(MusicNone)
	g.Audio.Play(SfxBoom)
	g.finish(false)
	g.Res.Killer = fmt.Sprintf("%v at (%.1f,%.1f) ship (%.1f,%.1f) speed %.0f", []string{"asteroid", "pylon", "beam"}[o.Kind], o.Pos.X, o.Pos.Y, g.Ship.X, g.Ship.Y, g.Speed)
}

func (g *Game) finish(survived bool) {
	g.Res = Result{Survived: survived, Score: g.Score, Hits: g.Hits, Spawned: g.Spawned, Shots: g.Shots,
		Time: g.elapsed(), BestChain: g.BestChain, Rank: -1}
	if survived {
		g.Res.Bonus = survivalPts
		g.Res.Score += survivalPts
		g.Score = g.Res.Score
		g.Audio.Music(MusicNone)
		g.Audio.Play(SfxComplete)
		g.setState(StFinish)
	} else {
		g.setState(StDying)
	}
	g.Res.Grade = grade(g.Res.Score, survived)
}

func grade(score int, survived bool) string {
	switch {
	case score >= 15000 && survived:
		return "S"
	case score >= 10000:
		return "A"
	case score >= 6000:
		return "B"
	case score >= 3000:
		return "C"
	}
	return "D"
}

func (g *Game) toResults() {
	g.Res.Rank = g.Scores.Add(g.Res)
	g.setState(StResults)
	g.Audio.Music(MusicTitle)
}
