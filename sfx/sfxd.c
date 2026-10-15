// sfxd — plays 44.1 kHz 16-bit stereo PCM from stdin on /dev/dsp.
//
// The game itself is a static Go binary, which can't use OnionOS's audio
// bridge (libpadsp.so, preloaded by launch.sh, routes OSS /dev/dsp writes to
// the audioserver). This tiny dynamically linked helper can, so the game
// pipes its mixed sound here. It exits when the game closes the pipe.
//
// Built with: zig cc -target arm-linux-gnueabihf.2.23 -mcpu=cortex_a7 -O2
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <sys/ioctl.h>
#include <unistd.h>

#define SNDCTL_DSP_SPEED       0xC0045002
#define SNDCTL_DSP_SETFMT      0xC0045005
#define SNDCTL_DSP_CHANNELS    0xC0045006
#define SNDCTL_DSP_SETFRAGMENT 0xC004500A
#define AFMT_S16_LE            0x00000010

int main(int argc, char **argv)
{
    const char *dev = argc > 1 ? argv[1] : "/dev/dsp";
    int fd = open(dev, O_WRONLY);
    if (fd < 0) {
        fprintf(stderr, "sfxd: can't open %s (errno %d)\n", dev, errno);
        return 1;
    }
    int frag = (4 << 16) | 11; // 4 fragments of 2 KB (~46 ms)
    int fmt = AFMT_S16_LE, ch = 2, rate = 44100;
    ioctl(fd, SNDCTL_DSP_SETFRAGMENT, &frag);
    ioctl(fd, SNDCTL_DSP_SETFMT, &fmt);
    ioctl(fd, SNDCTL_DSP_CHANNELS, &ch);
    ioctl(fd, SNDCTL_DSP_SPEED, &rate);
    fprintf(stderr, "sfxd: %s open (fmt %d, %d ch, %d Hz)\n", dev, fmt, ch, rate);

    static char buf[2048];
    for (;;) {
        ssize_t n = read(0, buf, sizeof buf);
        if (n < 0 && errno == EINTR)
            continue;
        if (n <= 0)
            break;
        char *p = buf;
        while (n > 0) {
            ssize_t w = write(fd, p, (size_t)n);
            if (w < 0) {
                if (errno == EINTR)
                    continue;
                fprintf(stderr, "sfxd: write failed (errno %d)\n", errno);
                close(fd);
                return 1;
            }
            p += w;
            n -= w;
        }
    }
    close(fd);
    return 0;
}
