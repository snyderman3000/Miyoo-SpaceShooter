package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"
)

const version = "1.0.0"
const tick = 1.0 / 60

func main() {
	shot := flag.String("shot", "", "render a scene to PNG instead of running on the device (title|countdown|play|dying|finish|results|pause)")
	at := flag.Float64("t", 20, "with -shot: seconds into the round")
	out := flag.String("out", "shot.png", "PNG path for -shot")
	w := flag.Int("w", 640, "screen width (desktop modes)")
	h := flag.Int("h", 480, "screen height (desktop modes)")
	seed := flag.Int64("seed", 1, "random seed (desktop modes)")
	bot := flag.Int("bot", 0, "play N rounds with the autopilot and print the results")
	wav := flag.String("wav", "", "with -bot: record the first round's audio to this WAV file")
	bench := flag.Int("bench", 0, "with -shot: render the scene N times and print the average frame time")
	frames := flag.String("frames", "", "with -bot: write a PNG every 6 ticks of the first round into this folder")
	audioTest := flag.String("audiotest", "", "stream a few seconds of game sound through this helper binary and exit")
	flag.Parse()

	if *audioTest != "" {
		a := NewAudio(true)
		a.StartDevice(*audioTest)
		a.Music(MusicPlay)
		for i := 0; i < 20; i++ {
			a.Play(Sfx(i % 9))
			time.Sleep(100 * time.Millisecond)
		}
		a.Close()
		fmt.Println("audiotest done")
		return
	}

	if *shot != "" || *bot > 0 {
		desktop(*shot, *at, *out, *w, *h, *seed, *bot, *wav, *bench, *frames)
		return
	}
	device()
}

func device() {
	exe, _ := os.Executable()
	appDir := filepath.Dir(exe)
	dataDir := filepath.Join(appDir, "data")
	os.MkdirAll(dataDir, 0o755)
	logf, _ := os.OpenFile(filepath.Join(dataDir, "log.txt"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if logf != nil {
		log.SetOutput(logf)
		defer logf.Close()
	}
	log.Printf("space shooter %s starting", version)
	runtime.GOMAXPROCS(2)
	debug.SetGCPercent(400)

	cfg := LoadConfig(dataDir)
	fb, err := OpenFramebuffer(cfg.PageFlip)
	if err != nil {
		log.Fatalf("framebuffer: %v", err)
	}
	defer fb.Close()
	keys := NewKeys()
	if err := keys.ReadInput("/dev/input/event0"); err != nil {
		log.Fatalf("input: %v", err)
	}
	audio := NewAudio(cfg.Music)
	audio.StartDevice(filepath.Join(appDir, "sfxd"))
	defer audio.Close()

	g := NewGame(buildModels(), time.Now().UnixNano(), audio, LoadScores(dataDir), cfg)
	view := NewView(fb.W, fb.H)

	last := time.Now()
	acc := 0.0
	statT := last
	nFrames := 0
	var renderTime time.Duration
	for !g.Quit {
	drain:
		for {
			select {
			case k := <-keys.Press:
				g.Press(k)
			default:
				break drain
			}
		}
		g.In = Input{
			Up: keys.Held(BtnUp), Down: keys.Held(BtnDown), Left: keys.Held(BtnLeft), Right: keys.Held(BtnRight),
			Fire:  keys.Held(BtnA) || keys.Held(BtnB),
			BankL: keys.Held(BtnL1) || keys.Held(BtnL2), BankR: keys.Held(BtnR1) || keys.Held(BtnR2),
		}
		now := time.Now()
		acc += math.Min(0.1, now.Sub(last).Seconds())
		last = now
		for acc >= tick {
			g.Update(tick)
			acc -= tick
		}
		t0 := time.Now()
		view.Render(g)
		fb.Present(view.Scr)
		renderTime += time.Since(t0)
		nFrames++
		if el := time.Since(statT); el > 10*time.Second {
			log.Printf("perf: %.1f fps, %.1f ms render+present, %d polys", float64(nFrames)/el.Seconds(),
				float64(renderTime.Microseconds())/1000/float64(nFrames), view.R.PolyStat)
			statT, nFrames, renderTime = time.Now(), 0, 0
		}
		// pace to ~60 fps
		if spare := time.Second/60 - time.Since(now); spare > time.Millisecond {
			time.Sleep(spare)
		}
	}
	log.Printf("bye")
}

// ---------- desktop test modes ----------

func desktop(shot string, at float64, out string, w, h int, seed int64, bots int, wav string, bench int, frames string) {
	tmp, _ := os.MkdirTemp("", "ss")
	defer os.RemoveAll(tmp)
	cfg := LoadConfig(tmp)
	models := buildModels()

	if shot != "" {
		scores := LoadScores("")
		scores.Entries = []ScoreEntry{{12400, 41, true, "Oct 14"}, {9800, 33, true, "Oct 14"}, {6100, 22, false, "Oct 13"}}
		g := NewGame(models, seed, nil, scores, cfg)
		view := NewView(w, h)
		if shot == "icon" {
			g.Dist = 2
			view.Icon(g)
			savePNG(view.Low, out)
			return
		}
		scene(g, shot, at)
		view.Render(g)
		if bench > 0 {
			t0 := time.Now()
			for i := 0; i < bench; i++ {
				g.Clock += tick
				view.Render(g)
			}
			fmt.Printf("%s: %.2f ms/frame, %d polys\n", shot, float64(time.Since(t0).Microseconds())/1000/float64(bench), view.R.PolyStat)
		}
		savePNG(view.Scr, out)
		return
	}

	var wf *os.File
	var wavBytes int
	if wav != "" {
		wf, _ = os.Create(wav)
		wf.Write(make([]byte, 44))
	}
	totals := struct{ score, hits, spawned, survived, shots int }{}
	var view *View
	if frames != "" {
		os.MkdirAll(frames, 0o755)
		view = NewView(w, h)
	}
	for run := 0; run < bots; run++ {
		var audio *Audio
		if run == 0 && wf != nil {
			audio = NewAudio(true)
		}
		g := NewGame(models, seed+int64(run), audio, LoadScores(""), cfg)
		buf := make([]int16, 736*2)
		bytes := make([]byte, len(buf)*2)
		step := func() {
			g.In = botInput(g)
			g.Update(tick)
			if audio != nil {
				audio.Fill(buf)
				for i, s := range buf {
					binary.LittleEndian.PutUint16(bytes[i*2:], uint16(s))
				}
				wf.Write(bytes)
				wavBytes += len(bytes)
			}
		}
		for i := 0; i < 60; i++ { // a second on the title screen
			step()
		}
		g.Press(BtnA)
		n := 0
		for g.State != StResults && n < 60*80 {
			step()
			if view != nil && run == 0 && n%6 == 0 {
				view.Render(g)
				savePNG(view.Scr, filepath.Join(frames, fmt.Sprintf("f%05d.png", n)))
			}
			n++
		}
		for i := 0; i < 90; i++ {
			step()
		}
		r := g.Res
		acc := 0.0
		if r.Shots > 0 {
			acc = 100 * float64(r.Hits) / float64(r.Shots)
		}
		fmt.Printf("run %d: %-9s score %5d  grade %s  hits %2d/%2d  shots %3d (%.0f%%)  chain %2d  time %.1fs\n",
			run, map[bool]string{true: "COMPLETE", false: "DESTROYED"}[r.Survived], r.Score, r.Grade, r.Hits, r.Spawned, r.Shots, acc, r.BestChain, r.Time)
		if r.Killer != "" {
			fmt.Println("        killed by", r.Killer)
		}
		totals.score += r.Score
		totals.hits += r.Hits
		totals.spawned += r.Spawned
		totals.shots += r.Shots
		if r.Survived {
			totals.survived++
		}
	}
	if bots > 1 {
		fmt.Printf("avg score %d, survived %d/%d, hit %d of %d targets\n", totals.score/bots, totals.survived, bots, totals.hits, totals.spawned)
	}
	if wf != nil {
		hdr := make([]byte, 44)
		copy(hdr, "RIFF")
		binary.LittleEndian.PutUint32(hdr[4:], uint32(36+wavBytes))
		copy(hdr[8:], "WAVEfmt ")
		binary.LittleEndian.PutUint32(hdr[16:], 16)
		binary.LittleEndian.PutUint16(hdr[20:], 1)
		binary.LittleEndian.PutUint16(hdr[22:], 2)
		binary.LittleEndian.PutUint32(hdr[24:], outRate)
		binary.LittleEndian.PutUint32(hdr[28:], outRate*4)
		binary.LittleEndian.PutUint16(hdr[32:], 4)
		binary.LittleEndian.PutUint16(hdr[34:], 16)
		copy(hdr[36:], "data")
		binary.LittleEndian.PutUint32(hdr[40:], uint32(wavBytes))
		wf.WriteAt(hdr, 0)
		wf.Close()
	}
}

func savePNG(c *Canvas, path string) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	png.Encode(f, c.ToImage())
	f.Close()
}

// scene fast-forwards a game (autopilot) to a state for screenshots.
func scene(g *Game, name string, at float64) {
	run := func(sec float64) {
		for i := 0; i < int(sec*60); i++ {
			g.In = botInput(g)
			g.Update(tick)
		}
	}
	switch name {
	case "title":
		run(2.3)
	case "countdown":
		run(1)
		g.Press(BtnA)
		run(1.2)
	case "play", "pause":
		g.God = true
		g.Press(BtnA)
		run(3 + at)
		if name == "pause" {
			g.Press(BtnStart)
		}
	case "dying", "results":
		g.Press(BtnA)
		run(3 + at)
		for _, o := range g.Objs { // fly into the nearest obstacle
			if !o.Kind.Target() && o.Pos.Z > 0 {
				o.Pos = g.Ship.Add(V3{0, 0, 0.2})
				break
			}
		}
		run(0.45)
		if name == "results" {
			run(4)
		}
	case "finish":
		g.God = true
		g.Press(BtnA)
		run(3 + 59.7)
		run(0.6)
	}
}
