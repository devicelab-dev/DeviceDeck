# demo

Record the one-take demo — stock Playwright driving a real simulator through
the DOM mirror — and turn it into a GIF for the README.

```bash
# 1. devicedeck serve is up, TestHive installed on a booted simulator
DEVICEDECK_UDID=<booted-udid> node demo/record-login.mjs
#    → writes demo/out/<hash>.webm

# 2. convert to a GIF (needs ffmpeg)
ffmpeg -i demo/out/*.webm -vf "fps=12,scale=360:-1:flags=lanczos" -loop 0 demo/login.gif
```

`?video=1` in the recording URL streams the device's own screen behind the
mirror, so the clip is the phone reacting to ordinary `getByTestId().click()`
and `keyboard.type()` calls — no mobile-specific code in sight.

The `.webm` and generated `.gif` are build artifacts, not checked in; the
script is, so the demo is reproducible.
