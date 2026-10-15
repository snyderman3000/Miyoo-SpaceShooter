#!/bin/sh
# Space Shooter — a 60-second polygon space shooter for OnionOS
appdir="$(cd "$(dirname "$0")" && pwd)"
cd "$appdir" || exit 1
mkdir -p data

# Sound: OnionOS sends OSS (/dev/dsp) audio to its audioserver when
# libpadsp.so is preloaded. The game pipes its sound to the small sfxd helper,
# which is the program that uses it.
for p in /customer/lib/libpadsp.so /mnt/SDCARD/miyoo/lib/libpadsp.so /mnt/SDCARD/.tmp_update/lib/libpadsp.so; do
    if [ -f "$p" ]; then export LD_PRELOAD="$p"; break; fi
done
export LD_LIBRARY_PATH="/config/lib:/customer/lib:$LD_LIBRARY_PATH"
echo "launch $(date): LD_PRELOAD=${LD_PRELOAD:-none} audioserver=$(ps | grep -c [a]udioserver)" > data/launch.log

touch /tmp/stay_awake
"$appdir/spaceshooter" >> data/launch.log 2>&1
echo "exit code $?" >> data/launch.log
rm -f /tmp/stay_awake
sync
