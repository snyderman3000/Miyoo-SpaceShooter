package main

// Miyoo Mini framebuffer + evdev input (same approach as Mixtape). The panel
// is mounted upside down, so frames are written rotated 180°. When the
// framebuffer has room for two pages the game flips between them to avoid
// tearing; otherwise it draws to the one visible page.

import (
	"encoding/binary"
	"io"
	"log"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

const (
	fbioGetVScreenInfo = 0x4600
	fbioPutVScreenInfo = 0x4601
	fbioGetFScreenInfo = 0x4602
	fbioPanDisplay     = 0x4606
)

type Framebuffer struct {
	f       *os.File
	mem     []byte
	px      []uint32
	W, H    int
	strideW int // in pixels
	flip    bool
	pages   int
	cur     int
	checked bool
	v       [40]uint32
}

func ioctl(fd uintptr, req uintptr, p unsafe.Pointer) error {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(p))
	if e != 0 {
		return e
	}
	return nil
}

func OpenFramebuffer(allowFlip bool) (*Framebuffer, error) {
	f, err := os.OpenFile("/dev/fb0", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	fb := &Framebuffer{f: f, W: 640, H: 480, flip: os.Getenv("SS_NOFLIP") == "", pages: 1}
	xvirt, yvirt := 640, 480
	if ioctl(f.Fd(), fbioGetVScreenInfo, unsafe.Pointer(&fb.v[0])) == nil && fb.v[0] > 0 {
		fb.W, fb.H = int(fb.v[0]), int(fb.v[1])
		xvirt, yvirt = int(fb.v[2]), int(fb.v[3])
		log.Printf("fb: %dx%d virtual %dx%d bpp %d yoffset %d", fb.W, fb.H, xvirt, yvirt, fb.v[6], fb.v[5])
		fb.v[5] = 0
		_ = ioctl(f.Fd(), fbioPutVScreenInfo, unsafe.Pointer(&fb.v[0]))
	}
	stride := xvirt * 4
	smem := stride * yvirt
	var fix [96]byte // struct fb_fix_screeninfo
	if ioctl(f.Fd(), fbioGetFScreenInfo, unsafe.Pointer(&fix[0])) == nil {
		ul := int(unsafe.Sizeof(uintptr(0)))
		sl := int(binary.LittleEndian.Uint32(fix[16+ul:]))
		lloff := (16 + ul + 4*4 + 2*3 + 3) &^ 3
		if ll := int(binary.LittleEndian.Uint32(fix[lloff:])); ll >= fb.W*4 {
			stride = ll
		}
		if sl >= stride*fb.H {
			smem = sl
		}
		log.Printf("fb: smem %d line %d", sl, stride)
	}
	if smem < stride*fb.H {
		smem = stride * fb.H
	}
	mem, err := syscall.Mmap(int(f.Fd()), 0, smem, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		return nil, err
	}
	fb.mem = mem
	fb.px = unsafe.Slice((*uint32)(unsafe.Pointer(&mem[0])), len(mem)/4)
	fb.strideW = stride / 4
	if allowFlip && yvirt >= 2*fb.H && smem >= 2*stride*fb.H {
		fb.pages = 2
	}
	log.Printf("fb: %d page(s)", fb.pages)
	return fb, nil
}

// Present copies the canvas to the framebuffer (rotated if needed) and, with
// two pages, flips to it.
func (fb *Framebuffer) Present(c *Canvas) {
	page := 0
	if fb.pages == 2 {
		page = 1 - fb.cur
	}
	base := page * fb.H * fb.strideW
	w, h := min(c.W, fb.W), min(c.H, fb.H)
	for y := 0; y < h; y++ {
		dst := fb.px[base+y*fb.strideW : base+y*fb.strideW+w]
		if fb.flip {
			sy := c.H - 1 - y
			src := c.Pix[sy*c.W : sy*c.W+c.W]
			n := c.W - 1
			for x := range dst {
				dst[x] = src[n-x]
			}
		} else {
			copy(dst, c.Pix[y*c.W:y*c.W+w])
		}
	}
	if fb.pages == 2 {
		fb.v[5] = uint32(page * fb.H)
		if err := ioctl(fb.f.Fd(), fbioPanDisplay, unsafe.Pointer(&fb.v[0])); err != nil {
			log.Printf("fb: pan failed (%v), using one page", err)
			fb.pages = 1
			fb.v[5] = 0
			_ = ioctl(fb.f.Fd(), fbioPutVScreenInfo, unsafe.Pointer(&fb.v[0]))
			copy(fb.px[:fb.H*fb.strideW], fb.px[base:base+fb.H*fb.strideW])
			return
		}
		if !fb.checked {
			// make sure the driver really moved to the page we drew
			var q [40]uint32
			if ioctl(fb.f.Fd(), fbioGetVScreenInfo, unsafe.Pointer(&q[0])) == nil && int(q[5]) != page*fb.H {
				log.Printf("fb: pan reported yoffset %d, want %d; using one page", q[5], page*fb.H)
				fb.pages = 1
				fb.v[5] = 0
				_ = ioctl(fb.f.Fd(), fbioPutVScreenInfo, unsafe.Pointer(&fb.v[0]))
				copy(fb.px[:fb.H*fb.strideW], fb.px[base:base+fb.H*fb.strideW])
				return
			}
			fb.checked = true
		}
		fb.cur = page
	}
}

// Close blanks the screen and leaves the display on page 0 for the menu.
func (fb *Framebuffer) Close() {
	for i := range fb.mem {
		fb.mem[i] = 0
	}
	fb.v[5] = 0
	_ = ioctl(fb.f.Fd(), fbioPutVScreenInfo, unsafe.Pointer(&fb.v[0]))
	syscall.Munmap(fb.mem)
	fb.f.Close()
}

// Miyoo button → Linux key code mapping.
const (
	BtnMenu   = 1   // ESC
	BtnL2     = 15  // TAB
	BtnR2     = 14  // BACKSPACE
	BtnL1     = 18  // E
	BtnR1     = 20  // T
	BtnStart  = 28  // ENTER
	BtnB      = 29  // LCTRL
	BtnX      = 42  // LSHIFT
	BtnY      = 56  // LALT
	BtnA      = 57  // SPACE
	BtnSelect = 97  // RCTRL
	BtnUp     = 103 //
	BtnLeft   = 105
	BtnRight  = 106
	BtnDown   = 108
	BtnPower  = 116
)

// Keys tracks which buttons are held and queues fresh presses.
type Keys struct {
	mu    sync.Mutex
	held  [256]bool
	Press chan int
}

func NewKeys() *Keys { return &Keys{Press: make(chan int, 32)} }

func (k *Keys) Held(code int) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return code >= 0 && code < 256 && k.held[code]
}

func (k *Keys) set(code int, down bool) {
	if code < 0 || code >= 256 {
		return
	}
	k.mu.Lock()
	k.held[code] = down
	k.mu.Unlock()
}

// ReadInput streams evdev key events from path.
func (k *Keys) ReadInput(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	go func() {
		defer f.Close()
		ev := 16
		if unsafe.Sizeof(uintptr(0)) == 8 {
			ev = 24
		}
		buf := make([]byte, ev)
		for {
			if _, err := io.ReadFull(f, buf); err != nil {
				return
			}
			o := ev - 8
			typ := binary.LittleEndian.Uint16(buf[o:])
			code := int(binary.LittleEndian.Uint16(buf[o+2:]))
			val := int32(binary.LittleEndian.Uint32(buf[o+4:]))
			if typ != 1 {
				continue
			}
			switch val {
			case 1:
				k.set(code, true)
				select {
				case k.Press <- code:
				default:
				}
			case 0:
				k.set(code, false)
			}
		}
	}()
	return nil
}
