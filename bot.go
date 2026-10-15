package main

import "math"

// botInput is a simple autopilot used by the desktop tests: dodge whatever is
// about to hit, otherwise line up with the nearest target and fire.
func botInput(g *Game) Input {
	var in Input
	if g.State != StPlay && g.State != StCountdown {
		return in
	}
	s := g.Ship
	clear := func(x, y float64) float64 {
		worst := 99.0
		for _, o := range g.Objs {
			if o.Kind.Target() || o.Pos.Z < s.Z-1.5 || o.Pos.Z > s.Z+30 {
				continue
			}
			var d float64
			switch o.Kind {
			case KAsteroid:
				d = math.Hypot(x-o.Pos.X, y-o.Pos.Y) - o.R - 1.7
			case KPylon:
				d = math.Abs(x-o.Pos.X) - o.Half.X - 1.9
			case KBeam:
				d = math.Abs(y-o.Pos.Y) - o.Half.Y - 0.9
			}
			// nearer obstacles matter more
			d += (o.Pos.Z - s.Z) * 0.04
			worst = math.Min(worst, d)
		}
		return worst
	}
	gx, gy := s.X, s.Y
	if clear(s.X, s.Y) < 0.3 {
		best := -99.0
		for dx := -5.0; dx <= 5; dx += 1 {
			for dy := -4.0; dy <= 4; dy += 1 {
				x, y := clampf(s.X+dx, -areaX, areaX), clampf(s.Y+dy, -areaY, areaY)
				c := math.Min(clear(x, y), 1.5) - math.Hypot(dx, dy)*0.12
				if c > best {
					best, gx, gy = c, x, y
				}
			}
		}
	} else {
		bestZ := 1e9
		for _, o := range g.Objs {
			if !o.Kind.Target() || o.Pos.Z < s.Z+8 || o.Pos.Z > s.Z+75 {
				continue
			}
			if clear(o.Pos.X, o.Pos.Y) < 0.6 {
				continue
			}
			if o.Pos.Z < bestZ {
				bestZ, gx, gy = o.Pos.Z, o.Pos.X, o.Pos.Y
			}
		}
	}
	const dead = 0.25
	in.Left = gx < s.X-dead
	in.Right = gx > s.X+dead
	in.Up = gy > s.Y+dead
	in.Down = gy < s.Y-dead
	for _, o := range g.Objs {
		if o.Kind.Target() && o.Pos.Z > s.Z+3 && o.Pos.Z < s.Z+90 &&
			math.Abs(o.Pos.X-s.X) < 1.4 && math.Abs(o.Pos.Y-s.Y) < 1.4 {
			in.Fire = true
		}
	}
	return in
}
