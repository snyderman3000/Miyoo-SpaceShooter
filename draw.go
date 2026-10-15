package main

import (
	"fmt"
	"math"
)

const floorY = -9.0
const camPitch = 0.14

// View holds the low-res 3D buffer, the full-res screen and the fonts.
type View struct {
	Low    *Canvas
	Scr    *Canvas
	R      *Renderer
	s      float64
	sky    []uint32
	floor  []uint32
	stars  [][3]int
	horizY int

	fLogo, fTitle, fBody, fSmall, fTiny, fLCD, fLCDBig, fHuge *Font
}

func NewView(w, h int) *View {
	v := &View{Scr: NewCanvas(w, h), Low: NewCanvas(w/2, h/2)}
	v.R = NewRenderer(v.Low)
	v.R.SetPitch(camPitch)
	v.s = math.Min(float64(w)/640, float64(h)/480)
	s := v.s
	v.fLogo = loadFont(titleTTF, 34*s)
	v.fTitle = loadFont(titleTTF, 21*s)
	v.fBody = loadFont(bodyTTF, 18*s)
	v.fSmall = loadFont(bodyTTF, 15*s)
	v.fTiny = loadFont(bodyTTF, 13*s)
	v.fLCD = loadFont(lcdTTF, 30*s)
	v.fLCDBig = loadFont(lcdTTF, 64*s)
	v.fHuge = loadFont(lcdTTF, 150*s)

	lw, lh := v.Low.W, v.Low.H
	v.horizY = int(v.R.Horizon())
	skyTop := rgb(0x06, 0x05, 0x10)
	skyHor := rgb(0x3c, 0x10, 0x4a)
	v.R.FogCol = skyHor
	v.sky = make([]uint32, lh)
	v.floor = make([]uint32, lh)
	for y := 0; y < lh; y++ {
		if y < v.horizY {
			t := float64(y) / float64(v.horizY)
			v.sky[y] = mix(skyTop, skyHor, t*t)
		} else {
			t := float64(y-v.horizY) / float64(lh-v.horizY)
			v.floor[y] = mix(rgb(0x1c, 0x08, 0x26), rgb(0x0a, 0x06, 0x10), math.Sqrt(t))
		}
	}
	// a fixed starfield above the horizon
	seed := uint32(12345)
	nx := func() int {
		seed ^= seed << 13
		seed ^= seed >> 17
		seed ^= seed << 5
		return int(seed >> 8)
	}
	for i := 0; i < 90; i++ {
		v.stars = append(v.stars, [3]int{nx() % (lw * 2), nx() % (v.horizY - 6), nx() % 100})
	}
	return v
}

func (v *View) px(n float64) int { return int(math.Round(n * v.s)) }

// background: sky, stars, the striped sun and the scrolling grid floor.
func (v *View) background(g *Game) {
	c := v.Low
	r := v.R
	w := c.W
	for y := 0; y < c.H; y++ {
		col := v.sky[y]
		if y >= v.horizY {
			col = v.floor[y]
		}
		row := c.Pix[y*w : (y+1)*w]
		for i := range row {
			row[i] = col
		}
	}
	camX := r.CamPos.X
	// stars drift a little with the camera
	for _, s := range v.stars {
		x := ((s[0]-int(camX*3))%(w*2) + w*2) % (w * 2)
		if x >= w {
			continue
		}
		b := 90 + (s[2]*7+int(g.Clock*40)*(s[2]%3))%120
		if s[2] < 12 {
			b = 230
		}
		c.Set(x, s[1], rgb(uint8(b), uint8(b), uint8(min(255, b+30))))
	}
	// sun
	rad := float64(c.H) * 0.18
	scx := r.CX - camX*1.2
	scy := float64(v.horizY) - rad*0.28
	top := int(scy - rad)
	for y := max(0, top); y < v.horizY; y++ {
		dy := float64(y) + 0.5 - scy
		if math.Abs(dy) > rad {
			continue
		}
		k := y - int(scy-rad*0.15)
		if k > 0 {
			gap := 1 + k/9
			if k%8 < gap {
				continue
			}
		}
		hw := math.Sqrt(rad*rad - dy*dy)
		t := (float64(y) - (scy - rad)) / (rad * 1.3)
		col := mix(rgb(0xff, 0xd0, 0x40), colMagenta, t)
		x0, x1 := int(scx-hw), int(scx+hw)
		for x := max(0, x0); x < min(w, x1); x++ {
			c.Pix[y*w+x] = col
		}
	}
	// horizon glow
	if v.horizY < c.H {
		row := c.Pix[v.horizY*w : (v.horizY+1)*w]
		for i := range row {
			row[i] = mix(colMagenta, v.floor[v.horizY], 0.25)
		}
	}
	// grid floor: lines along Z at fixed X, and cross lines scrolling with distance
	hy := floorY - r.CamPos.Y
	gridCol := colAmber
	for k := -14; k <= 14; k++ {
		x := float64(k)*4 - camX
		zs := []float64{2, 9, 24, 60, 150}
		for i := 0; i+1 < len(zs); i++ {
			ax, ay := r.Project(r.View(V3{x, hy, zs[i]}))
			bx, by := r.Project(r.View(V3{x, hy, zs[i+1]}))
			col := r.fog(mix(gridCol, v.floor[min(c.H-1, int(ay))], 0.5), zs[i])
			c.Line(int(ax), int(ay), int(bx), int(by), col)
		}
	}
	off := math.Mod(g.Dist, 6)
	lastY := -10
	for n := 25; n >= 0; n-- { // far to near, skipping lines that would pile up
		z := float64(n)*6 - off + 3
		if z < 2 {
			continue
		}
		_, y := r.Project(r.View(V3{0, hy, z}))
		yi := int(y)
		if yi <= v.horizY+1 || yi >= c.H || yi-lastY < 3 {
			continue
		}
		lastY = yi
		col := r.fog(mix(gridCol, v.floor[yi], 0.5), z)
		row := c.Pix[yi*w : (yi+1)*w]
		for i := range row {
			row[i] = col
		}
	}
}

func (v *View) camera(g *Game) {
	shx, shy := 0.0, 0.0
	if g.Shake > 0 {
		shx = math.Sin(g.Clock*61) * g.Shake * 0.35
		shy = math.Cos(g.Clock*47) * g.Shake * 0.3
	}
	if g.State == StTitle {
		v.R.CamPos = V3{0, 1.15, -10.5}
		return
	}
	v.R.CamPos = V3{g.Ship.X*0.62 + shx, g.Ship.Y*0.55 + 1.95 + shy, -10.5}
}

// Icon draws the app icon (ship over the sunset) into v.Low.
func (v *View) Icon(g *Game) {
	v.R.CamPos = V3{0, 1.95, -10.5}
	v.background(g)
	v.R.Begin()
	rot := Rot(-0.3, 0.55, -0.4)
	v.R.Mesh(g.M.Ship, V3{0.2, 1.45, -6.3}, &rot, 1.15, 0)
	v.R.Flush()
}

// Render draws a complete frame into v.Scr.
func (v *View) Render(g *Game) {
	v.camera(g)
	v.background(g)
	r := v.R
	r.Begin()
	for _, o := range g.Objs {
		rot := Rot(o.Ang.X, o.Ang.Y, o.Ang.Z)
		r.Mesh(o.Mesh, o.Pos, &rot, o.S, math.Max(0, o.Flash))
	}
	for _, p := range g.Parts {
		if r.View(p.Pos.Sub(r.CamPos)).Z < 4 {
			continue // debris right in front of the lens just blots out the screen
		}
		t := p.Life / p.Max
		if p.Mesh != nil {
			rot := Rot(p.Ang.X, p.Ang.Y, p.Ang.Z)
			r.Mesh(p.Mesh, p.Pos, &rot, p.Size*(0.4+0.6*t), 0)
		} else {
			r.Billboard(p.Pos, p.Size*(0.5+t), mix(colAmber, p.Col, t), false)
		}
	}
	for _, b := range g.Bolts {
		r.Beam(b.Pos, b.Pos.Sub(V3{0, 0, 4}), 0.17, colGreen)
		r.Billboard(b.Pos, 0.16, rgb(0xe8, 0xff, 0xe0), false)
	}
	if g.State == StTitle {
		rot := Rot(-0.28, g.Clock*0.7, math.Sin(g.Clock*0.9)*0.25)
		r.Mesh(g.M.Ship, V3{0, 1.5, -3.0}, &rot, 0.75, 0)
	} else if g.ShipAlive {
		rot := Rot(g.Pitch-0.06, g.Yaw, g.Bank)
		r.Mesh(g.M.Ship, g.Ship, &rot, shipScale, 0)
		tail := g.Ship.Add(rot.Apply(V3{0, 0.1, -1.3}.Mul(shipScale)))
		fl := 0.26 + 0.07*math.Sin(g.Clock*53)
		if g.State == StFinish {
			fl *= 1.8
		}
		r.Billboard(tail, fl, colMagenta, false)
		r.Billboard(tail.Sub(V3{0, 0, 0.15}), fl*0.5, colPaper, false)
	}
	r.Flush()

	v.Scr.Upscale2x(v.Low)
	c := v.Scr
	k := float64(c.W) / float64(v.Low.W)

	// shockwave rings
	for _, ring := range g.Rings {
		x, y, ok := r.ProjectWorld(ring.Pos)
		if !ok {
			continue
		}
		t := ring.T / ring.Max
		z := r.View(ring.Pos.Sub(r.CamPos)).Z
		rad := int(r.F * ring.Size * (0.3 + t) / z * k)
		if rad > 0 && rad < c.W/2 {
			c.Circle(int(x*k), int(y*k), rad, mix(ring.Col, colBG, t*0.8), false, max(1, v.px(3*(1-t))))
		}
	}
	// score popups
	for _, p := range g.Pops {
		x, y, ok := r.ProjectWorld(p.Pos)
		if !ok {
			continue
		}
		t := p.T / 0.9
		c.TextCenter(v.fLCD, int(x*k)+1, int(y*k)-v.px(20+50*t)+1, p.Text, colBG)
		c.TextCenter(v.fLCD, int(x*k), int(y*k)-v.px(20+50*t), p.Text, mix(p.Col, colPaperDim, t))
	}
	if g.FlashT > 0 {
		c.Blend(0, 0, c.W, c.H, colPaper, g.FlashT/0.35*0.8)
	}

	switch g.State {
	case StTitle:
		v.title(g)
	case StCountdown, StPlay:
		v.reticle(g)
		v.hud(g)
		if g.State == StCountdown {
			v.countdown(g)
		} else if g.T < 0.8 {
			v.bigCenter("GO!", colCyan, 1-g.T/0.8)
		}
	case StDying:
		v.hud(g)
		if g.T > 0.6 {
			v.banner("SHIP DESTROYED", colMagenta)
		}
	case StFinish:
		v.hud(g)
		v.banner("TIME UP", colCyan)
	case StResults:
		v.results(g)
	}
	if g.Paused {
		v.pause(g)
	}
}

func (v *View) reticle(g *Game) {
	if !g.ShipAlive {
		return
	}
	k := float64(v.Scr.W) / float64(v.Low.W)
	for i, d := range []float64{20, 42} {
		x, y, ok := v.R.ProjectWorld(g.Ship.Add(V3{0, -0.05, d}))
		if !ok {
			continue
		}
		sz := v.px(30 - float64(i)*12)
		col := colGreen
		if i == 1 {
			col = mix(colGreen, colBG, 0.35)
		}
		v.Scr.Corners(int(x*k)-sz/2, int(y*k)-sz/2, sz, sz, max(4, sz/3), max(1, v.px(2)), col)
	}
}

// header: Mixtape's top bar — logo left, readouts right, tape stripe under it.
func (v *View) header(sub string) int {
	c := v.Scr
	h := v.px(46)
	c.Blend(0, 0, c.W, h, colBG, 0.78)
	x := c.TextGlow(v.fTitle, v.px(14), v.px(11), "SPACE SHOOTER", colAmber)
	if sub != "" {
		c.Text(v.fTiny, x+v.px(10), v.px(17), sub, colCyan)
	}
	c.Stripe(0, h, c.W, max(1, v.px(2)))
	return h
}

func (v *View) hud(g *Game) {
	c := v.Scr
	h := v.px(46)
	c.Blend(0, 0, c.W, h, colBG, 0.72)
	c.Stripe(0, h, c.W, max(1, v.px(2)))
	// score
	c.Text(v.fTiny, v.px(14), v.px(4), "SCORE", colPaperDim)
	c.TextGlow(v.fLCD, v.px(14), v.px(14), fmt.Sprintf("%06d", g.Score), colAmber)
	// timer, centre
	tl := g.TimeLeft
	secs := int(math.Ceil(tl))
	col := colAmber
	if secs <= 10 && g.State == StPlay {
		col = colMagenta
		if math.Mod(tl, 1) < 0.25 {
			col = mix(colMagenta, colPaper, 0.5)
		}
	}
	ts := fmt.Sprintf("%d:%02d", secs/60, secs%60)
	tw := v.fLCDBig.Width(ts)
	bx := c.W/2 - tw/2 - v.px(14)
	c.Fill(bx, 0, tw+v.px(28), v.px(56), colPanel)
	c.Border(bx, 0, tw+v.px(28), v.px(56), 1, colAmberDim)
	c.TextGlow(v.fLCDBig, c.W/2-tw/2, v.px(-6), ts, col)
	// time bar under the stripe
	frac := clampf(tl/roundTime, 0, 1)
	by := h + 3*max(1, v.px(2))
	c.Fill(0, by, int(float64(c.W)*frac), max(1, v.px(3)), mix(col, colBG, 0.35))
	// hits and chain, right
	right := c.W - v.px(14)
	c.TextRight(v.fTiny, right, v.px(4), "TARGETS", colPaperDim)
	c.TextRight(v.fLCD, right, v.px(14), fmt.Sprintf("%02d", g.Hits), colCyan)
	if g.Mult > 1 {
		s := fmt.Sprintf("CHAIN x%d", g.Mult)
		w := v.fSmall.Width(s) + 12
		c.Chip(v.fSmall, right-w+6, by+v.px(10), s, colMagenta, true)
	}
}

func (v *View) countdown(g *Game) {
	n := 3 - int(g.T)
	if n < 1 {
		n = 1
	}
	f := 1 - math.Mod(g.T, 1)
	v.bigCenter(fmt.Sprint(n), colAmber, f)
	c := v.Scr
	c.TextCenter(v.fBody, c.W/2, c.H-v.px(70), "SHOOT THE DRONES  //  DODGE EVERYTHING ELSE", colPaper)
}

func (v *View) bigCenter(s string, col uint32, fade float64) {
	c := v.Scr
	y := c.H/2 - v.fHuge.height/2 - v.px(70)
	c.TextCenter(v.fHuge, c.W/2+v.px(3), y+v.px(3), s, colBG)
	cc := mix(colBG, col, 0.35+0.65*fade)
	w := v.fHuge.Width(s)
	c.TextGlow(v.fHuge, c.W/2-w/2, y, s, cc)
}

func (v *View) banner(s string, col uint32) {
	c := v.Scr
	h := v.px(64)
	y := c.H/2 - h/2
	c.Blend(0, y, c.W, h, colBG, 0.7)
	c.Fill(0, y, c.W, max(1, v.px(2)), col)
	c.Fill(0, y+h-max(1, v.px(2)), c.W, max(1, v.px(2)), col)
	w := v.fLogo.Width(s)
	c.TextGlow(v.fLogo, c.W/2-w/2, y+h/2-v.fLogo.height/2, s, col)
}

func (v *View) footer(btns [][2]string, right string) {
	c := v.Scr
	h := v.px(32)
	y := c.H - h
	c.Blend(0, y, c.W, h, colBG, 0.85)
	c.Fill(0, y, c.W, 1, colGrid)
	x := v.px(12)
	for i, b := range btns {
		col := colAmber
		if i == 1 {
			col = colPaperDim
		}
		if i == 2 {
			col = colCyan
		}
		x = c.Button(v.fSmall, x, y+v.px(7), b[0], b[1], col)
	}
	if right != "" {
		c.TextRight(v.fSmall, c.W-v.px(12), y+v.px(8), right, colPaperDim)
	}
}

func (v *View) title(g *Game) {
	c := v.Scr
	v.header("// 60-SECOND SORTIE")
	best := g.Scores.Best()
	c.TextRight(v.fTiny, c.W-v.px(14), v.px(6), "HI-SCORE", colPaperDim)
	c.TextRight(v.fLCD, c.W-v.px(14), v.px(14), fmt.Sprintf("%06d", best), colAmber)

	// mission card, left
	pw, ph := v.px(270), v.px(150)
	px, py := v.px(16), c.H-v.px(32)-v.px(52)-ph
	c.Blend(px, py, pw, ph, colPanel, 0.82)
	c.Corners(px, py, pw, ph, v.px(12), max(1, v.px(2)), colAmberDim)
	c.Text(v.fTiny, px+v.px(12), py+v.px(10), "MISSION", colPaperDim)
	lines := []struct {
		s   string
		col uint32
	}{
		{"60 SECONDS ON THE CLOCK", colPaper},
		{"SHOOT DRONES   +100", colCyan},
		{"GOLD CUBES     +300", colAmber},
		{"QUICK HITS CHAIN UP TO x4", colPaper},
		{"HIT ANYTHING ELSE: BOOM", colMagenta},
		{"SURVIVE        +1000", colGreen},
	}
	for i, l := range lines {
		c.Text(v.fSmall, px+v.px(12), py+v.px(30)+i*v.px(19), l.s, l.col)
	}

	// top scores, right
	qx, qw := c.W-v.px(16)-v.px(270), v.px(270)
	qh := v.px(150)
	c.Blend(qx, py, qw, qh, colPanel, 0.82)
	c.Corners(qx, py, qw, qh, v.px(12), max(1, v.px(2)), colAmberDim)
	c.Text(v.fTiny, qx+v.px(12), py+v.px(10), "TOP SCORES", colPaperDim)
	es := g.Scores.Entries
	for i := 0; i < 5; i++ {
		y := py + v.px(28) + i*v.px(23)
		c.Text(v.fLCD, qx+v.px(12), y-v.px(5), fmt.Sprintf("%d", i+1), colAmberDim)
		if i < len(es) {
			e := es[i]
			c.Text(v.fLCD, qx+v.px(36), y-v.px(5), fmt.Sprintf("%06d", e.Score), colAmber)
			mark, mc := "OK", colGreen
			if !e.Survived {
				mark, mc = "KO", colMagenta
			}
			c.Text(v.fTiny, qx+v.px(140), y+v.px(3), mark, mc)
			c.TextRight(v.fTiny, qx+qw-v.px(10), y+v.px(3), e.Date, colPaperDim)
		} else {
			c.Text(v.fLCD, qx+v.px(36), y-v.px(5), "------", colGrid)
		}
	}

	if int(g.Clock*2)%2 == 0 {
		s := "PRESS  A  TO LAUNCH"
		c.TextCenter(v.fTitle, c.W/2+1, c.H-v.px(72)+1, s, colBG)
		c.TextCenter(v.fTitle, c.W/2, c.H-v.px(72), s, colAmber)
	}
	music := "MUSIC ON"
	if !g.Cfg.Music {
		music = "MUSIC OFF"
	}
	v.footer([][2]string{{"A", "LAUNCH"}, {"MENU", "EXIT"}, {"SELECT", music}}, "v"+version)
}

func (v *View) results(g *Game) {
	c := v.Scr
	v.header("// DEBRIEF")
	res := g.Res
	pw, ph := v.px(420), v.px(318)
	px, py := c.W/2-pw/2, v.px(64)
	c.Blend(px, py, pw, ph, colPanel, 0.9)
	col := colCyan
	title := "MISSION COMPLETE"
	if !res.Survived {
		col = colMagenta
		title = "SHIP DESTROYED"
	}
	c.Corners(px, py, pw, ph, v.px(16), max(1, v.px(2)), col)
	c.TextGlow(v.fTitle, px+v.px(20), py+v.px(16), title, col)
	// grade box
	gs := v.px(86)
	gx, gy := px+pw-gs-v.px(20), py+v.px(16)
	c.Fill(gx, gy, gs, gs, colPanel2)
	c.Border(gx, gy, gs, gs, max(1, v.px(2)), colAmber)
	c.Text(v.fTiny, gx+v.px(6), gy+v.px(4), "RANK", colPaperDim)
	gw := v.fLCDBig.Width(res.Grade)
	c.TextGlow(v.fLCDBig, gx+gs/2-gw/2, gy+v.px(12), res.Grade, colAmber)

	c.Text(v.fTiny, px+v.px(20), py+v.px(52), "FINAL SCORE", colPaperDim)
	c.TextGlow(v.fLCDBig, px+v.px(18), py+v.px(56), fmt.Sprintf("%06d", res.Score), colAmber)

	acc := 0
	if res.Shots > 0 {
		acc = int(math.Round(100 * float64(res.Hits) / float64(res.Shots)))
	}
	rows := [][3]string{
		{"TARGETS", fmt.Sprintf("%d / %d", res.Hits, res.Spawned), ""},
		{"ACCURACY", fmt.Sprintf("%d%%", min(acc, 100)), ""},
		{"BEST CHAIN", fmt.Sprintf("%d HITS", res.BestChain), ""},
		{"FLIGHT TIME", fmt.Sprintf("%.1f s", res.Time), ""},
		{"SURVIVAL", fmt.Sprintf("+%d", res.Bonus), ""},
	}
	ry := py + v.px(132)
	for i, rw := range rows {
		y := ry + i*v.px(24)
		c.Text(v.fBody, px+v.px(20), y, rw[0], colPaperDim)
		vc := colAmber
		if i == 4 {
			vc = colGreen
			if res.Bonus == 0 {
				vc = colMagenta
			}
		}
		c.Text(v.fBody, px+v.px(190), y, rw[1], vc)
	}
	c.HLineDashed(px+v.px(20), py+ph-v.px(52), pw-v.px(40), colGrid)
	if res.Rank == 0 {
		if int(g.Clock*3)%2 == 0 {
			c.Chip(v.fBody, px+v.px(20), py+ph-v.px(40), "NEW HIGH SCORE", colMagenta, true)
		}
	} else if res.Rank > 0 {
		c.Chip(v.fBody, px+v.px(20), py+ph-v.px(40), fmt.Sprintf("TOP SCORE #%d", res.Rank+1), colCyan, false)
	} else {
		c.Text(v.fSmall, px+v.px(20), py+ph-v.px(36), fmt.Sprintf("HI-SCORE %06d", g.Scores.Best()), colPaperDim)
	}
	if g.T >= 0.8 {
		v.footer([][2]string{{"A", "FLY AGAIN"}, {"B", "TITLE"}, {"MENU", "EXIT"}}, "")
	}
}

func (v *View) pause(g *Game) {
	c := v.Scr
	c.Blend(0, 0, c.W, c.H, colBG, 0.55)
	pw, ph := v.px(300), v.px(150)
	px, py := c.W/2-pw/2, c.H/2-ph/2
	c.Fill(px, py, pw, ph, colPanel)
	c.Corners(px, py, pw, ph, v.px(14), max(1, v.px(2)), colAmber)
	c.TextCenter(v.fLogo, c.W/2, py+v.px(18), "PAUSED", colAmber)
	c.Stripe(px+v.px(30), py+v.px(64), pw-v.px(60), max(1, v.px(2)))
	music := "MUSIC ON"
	if !g.Cfg.Music {
		music = "MUSIC OFF"
	}
	c.TextCenter(v.fSmall, c.W/2, py+v.px(84), "A RESUME   B QUIT TO TITLE", colPaper)
	c.TextCenter(v.fSmall, c.W/2, py+v.px(108), "MENU EXIT   SELECT "+music, colPaperDim)
}

func (c *Canvas) HLineDashed(x, y, w int, col uint32) {
	for i := x; i < x+w; i += 8 {
		c.Fill(i, y, min(4, x+w-i), 1, col)
	}
}
