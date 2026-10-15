package main

// Synthesised sound: every effect and the synthwave music loop are generated
// here, mixed to 44.1 kHz 16-bit stereo, and handed to the sfxd helper (a
// tiny C program that writes to OnionOS's /dev/dsp). A static Go binary can't
// use OnionOS's libpadsp.so audio bridge itself, the helper can. Without the
// helper the game simply runs silent.

import (
	"encoding/binary"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const (
	sampleRate = 22050 // synthesis rate
	outRate    = 44100 // what the helper plays (each sample is doubled)
)

type Sfx int

const (
	SfxLaser Sfx = iota
	SfxHit
	SfxBonus
	SfxClink
	SfxBoom
	SfxBeep
	SfxGo
	SfxTick
	SfxComplete
)

type MusicMode int

const (
	MusicNone MusicMode = iota
	MusicTitle
	MusicPlay
)

type voice struct {
	kind  Sfx
	t     float64 // seconds since start
	phase float64
	lp    float64
	pan   float64
}

type Audio struct {
	mu       sync.Mutex
	pending  []Sfx
	voices   []voice
	music    MusicMode
	musicOn  bool
	paused   bool
	mstep    int     // music step (16ths)
	mpos     float64 // samples into the current step
	bassPh   float64
	leadPh   float64
	kickT    float64
	snareT   float64
	hatT     float64
	bassLP   float64
	noise    uint32
	out      io.WriteCloser
	cmd      *exec.Cmd
	stop     chan struct{}
	stopped  sync.WaitGroup
	Disabled bool
}

func NewAudio(musicOn bool) *Audio {
	return &Audio{musicOn: musicOn, noise: 22222, stop: make(chan struct{})}
}

// StartDevice launches the helper and streams audio to it in the background.
func (a *Audio) StartDevice(helper string) {
	if _, err := os.Stat(helper); err != nil {
		log.Printf("audio: no helper at %s, running silent", helper)
		return
	}
	r, w, err := os.Pipe()
	if err != nil {
		log.Printf("audio: pipe: %v", err)
		return
	}
	// keep the pipe small so sound stays in step with the picture (~23 ms)
	const fSetPipeSz = 1031
	if _, _, e := syscall.Syscall(syscall.SYS_FCNTL, w.Fd(), fSetPipeSz, 4096); e != 0 {
		log.Printf("audio: F_SETPIPE_SZ: %v", e)
	}
	var args []string
	if d := os.Getenv("SS_DSP"); d != "" { // tests: write somewhere other than /dev/dsp
		args = append(args, d)
	}
	cmd := exec.Command(helper, args...)
	cmd.Stdin = r
	cmd.Stderr = log.Writer()
	if err := cmd.Start(); err != nil {
		log.Printf("audio: helper: %v", err)
		r.Close()
		w.Close()
		return
	}
	r.Close()
	a.out, a.cmd = w, cmd
	log.Printf("audio: helper started (pid %d)", cmd.Process.Pid)
	a.stopped.Add(1)
	go func() {
		defer a.stopped.Done()
		buf := make([]int16, 512*2)
		bytes := make([]byte, len(buf)*2)
		start := time.Now()
		sent := 0.0 // seconds of audio handed to the helper
		for {
			// never run more than ~80 ms ahead of real time, even if the
			// device accepts data faster than it plays it
			if ahead := sent - time.Since(start).Seconds(); ahead > 0.08 {
				time.Sleep(time.Duration((ahead - 0.06) * float64(time.Second)))
			} else if ahead < -0.5 {
				start = time.Now().Add(-time.Duration(sent * float64(time.Second))) // resync after a stall
			}
			sent += float64(len(buf)/2) / outRate
			select {
			case <-a.stop:
				return
			default:
			}
			a.Fill(buf)
			for i, s := range buf {
				binary.LittleEndian.PutUint16(bytes[i*2:], uint16(s))
			}
			if _, err := w.Write(bytes); err != nil {
				log.Printf("audio: helper stopped (%v), running silent", err)
				return
			}
		}
	}()
}

func (a *Audio) Close() {
	if a == nil || a.out == nil {
		return
	}
	close(a.stop)
	a.stopped.Wait()
	a.out.Close()
	if a.cmd != nil {
		a.cmd.Wait()
	}
}

func (a *Audio) Play(s Sfx) {
	if a == nil {
		return
	}
	a.mu.Lock()
	if len(a.pending) < 32 {
		a.pending = append(a.pending, s)
	}
	a.mu.Unlock()
}

func (a *Audio) Music(m MusicMode) {
	if a == nil {
		return
	}
	a.mu.Lock()
	if a.music != m {
		a.music = m
		a.mstep, a.mpos = 0, 0
	}
	a.mu.Unlock()
}

func (a *Audio) SetMusicOn(on bool) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.musicOn = on
	a.mu.Unlock()
}

func (a *Audio) Pause(p bool) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.paused = p
	a.mu.Unlock()
}

func (a *Audio) rnd() float64 {
	a.noise ^= a.noise << 13
	a.noise ^= a.noise >> 17
	a.noise ^= a.noise << 5
	return float64(a.noise)/float64(1<<31) - 1
}

var sfxLen = map[Sfx]float64{
	SfxLaser: 0.12, SfxHit: 0.42, SfxBonus: 0.34, SfxClink: 0.07, SfxBoom: 1.8,
	SfxBeep: 0.14, SfxGo: 0.42, SfxTick: 0.05, SfxComplete: 1.1,
}

func sq(ph float64) float64 {
	if math.Mod(ph, 1) < 0.5 {
		return 1
	}
	return -1
}

func tri(ph float64) float64 {
	f := math.Mod(ph, 1)
	return 4*math.Abs(f-0.5) - 1
}

var noteTable = func() (t [128]float64) {
	for i := range t {
		t[i] = 440 * math.Pow(2, float64(i-69)/12)
	}
	return
}()

var arpOrder = [6]int{0, 1, 2, 3, 2, 1}

func noteHz(n int) float64 { return noteTable[n&127] }

// sample renders one voice; returns value and whether it is finished.
func (a *Audio) sample(v *voice) (float64, bool) {
	dt := 1.0 / sampleRate
	t := v.t
	if t >= sfxLen[v.kind] {
		return 0, true
	}
	var s float64
	switch v.kind {
	case SfxLaser:
		f := 1700*math.Exp(-t*26) + 380
		v.phase += f * dt
		e := 1 - t/0.12
		env := e * e
		s = 0.16 * sq(v.phase) * env
	case SfxHit:
		f := 720*math.Exp(-t*9) + 90
		v.phase += f * dt
		n := a.rnd()
		v.lp += (n - v.lp) * 0.35
		env := math.Exp(-t * 9)
		bl := 0.0
		if t < 0.09 {
			bl = 0.12 * sq(t*1320) * (1 - t/0.09)
		}
		s = (0.28*v.lp+0.22*math.Sin(2*math.Pi*v.phase))*env + bl
	case SfxBonus:
		notes := []int{84, 88, 91, 96}
		i := min(3, int(t/0.06))
		v.phase += noteHz(notes[i]) * dt
		s = 0.13 * sq(v.phase) * (1 - t/0.34)
	case SfxClink:
		v.phase += 2600 * dt
		s = 0.08 * (sq(v.phase)*0.6 + a.rnd()*0.4) * (1 - t/0.07)
	case SfxBoom:
		n := a.rnd()
		cut := 0.05 + 0.5*math.Exp(-t*3)
		v.lp += (n - v.lp) * cut
		v.phase += (40 + 60*math.Exp(-t*4)) * dt
		env := math.Exp(-t * 2.2)
		s = (0.6*v.lp + 0.35*math.Sin(2*math.Pi*v.phase)) * env
	case SfxBeep:
		v.phase += 660 * dt
		s = 0.2 * math.Sin(2*math.Pi*v.phase) * (1 - t/0.14)
	case SfxGo:
		v.phase += 990 * dt
		s = 0.2 * (0.7*math.Sin(2*math.Pi*v.phase) + 0.3*sq(v.phase)) * (1 - t/0.42)
	case SfxTick:
		v.phase += 1250 * dt
		s = 0.1 * sq(v.phase) * (1 - t/0.05)
	case SfxComplete:
		notes := []int{69, 72, 76, 81, 84, 88}
		i := min(5, int(t/0.09))
		v.phase += noteHz(notes[i]) * dt
		env := 1.0
		if t > 0.5 {
			env = 1 - (t-0.5)/0.6
		}
		s = 0.12 * (sq(v.phase)*0.5 + tri(v.phase)*0.5) * env
	}
	v.t += dt
	return s, false
}

// Chords (A minor, F, C, G) — root MIDI notes for the bass, 4 bars of 16ths.
var chordRoots = []int{45, 41, 48, 43}
var chordTones = [][]int{{57, 60, 64, 69}, {53, 57, 60, 65}, {55, 60, 64, 67}, {55, 59, 62, 67}}

const stepSamples = sampleRate * 60 / 116 / 4

// musicSample renders one sample of the loop.
func (a *Audio) musicSample() float64 {
	if a.music == MusicNone || !a.musicOn {
		return 0
	}
	dt := 1.0 / sampleRate
	if a.mpos == 0 {
		st := a.mstep % 16
		if st%4 == 0 {
			a.kickT = 0
		}
		if st == 4 || st == 12 {
			a.snareT = 0
		}
		if st%4 == 2 {
			a.hatT = 0
		}
	}
	bar := (a.mstep / 16) % 4
	st := a.mstep % 16
	var s float64
	// bass: driving 8ths, root and octave
	root := chordRoots[bar]
	if (st/2)%2 == 1 {
		root += 12
	}
	a.bassPh += noteHz(root) * dt
	saw := 2*math.Mod(a.bassPh, 1) - 1
	a.bassLP += (saw - a.bassLP) * 0.12
	gate := 1 - math.Mod(a.mpos+float64((st%2)*stepSamples), 2*stepSamples)/(2*stepSamples)*0.7
	s += 0.15 * a.bassLP * gate
	// drums
	if a.kickT < 0.18 {
		f := 50 + 110*math.Exp(-a.kickT*30)
		s += 0.32 * math.Sin(2*math.Pi*f*a.kickT) * math.Exp(-a.kickT*16)
		a.kickT += dt
	}
	if a.music == MusicPlay {
		if a.snareT < 0.14 {
			s += 0.11 * a.rnd() * math.Exp(-a.snareT*24)
			a.snareT += dt
		}
		if a.hatT < 0.04 {
			s += 0.05 * a.rnd() * (1 - a.hatT/0.04)
			a.hatT += dt
		}
		// arpeggio lead on 16ths
		tones := chordTones[bar]
		n := tones[arpOrder[st%6]] + 12
		a.leadPh += noteHz(n) * dt
		env := math.Exp(-a.mpos / stepSamples * 3)
		s += 0.05 * tri(a.leadPh) * env
	}
	a.mpos++
	if a.mpos >= stepSamples {
		a.mpos = 0
		a.mstep++
	}
	return s * 0.85
}

// Fill mixes interleaved stereo samples into buf.
func (a *Audio) Fill(buf []int16) {
	a.mu.Lock()
	for _, s := range a.pending {
		// the laser is fired constantly: keep only a few of them going
		if s == SfxLaser {
			n := 0
			for i := range a.voices {
				if a.voices[i].kind == SfxLaser {
					n++
				}
			}
			if n >= 2 {
				continue
			}
		}
		a.voices = append(a.voices, voice{kind: s})
	}
	a.pending = a.pending[:0]
	paused := a.paused
	for i := 0; i+3 < len(buf); i += 4 {
		var s float64
		if !paused {
			s = a.musicSample()
			live := a.voices[:0]
			for vi := range a.voices {
				v := a.voices[vi]
				x, done := a.sample(&v)
				s += x
				if !done {
					live = append(live, v)
				}
			}
			a.voices = live
		}
		// soft clip
		s = s / (1 + math.Abs(s)*0.6)
		v := int16(clampf(s, -1, 1) * 30000)
		buf[i], buf[i+1], buf[i+2], buf[i+3] = v, v, v, v
	}
	a.mu.Unlock()
}
