# Avantis Utils

A web-based utility for integrating **MixingStation**, **ProPresenter**, and the **Allen & Heath Avantis** console.

## Features

- **Mic monitoring** — 8 toggle buttons (green = active, red = inactive). Toggling a mic off triggers a "Clear Mics" macro in ProPresenter; toggling on fires its individual `*_On` macro.
- **Tap-rate detection** — Select an audio capture device, press **Set tap rate**, and the app listens for 5 seconds, measures the click-track tempo (median peak-to-peak interval), converts it to milliseconds, and sends it to the Avantis delay FX (`fx.rack.0.tl` by default, configurable). The result is shown as both **ms** and **BPM** on the page.

---

## Quick start

```bash
# Build (requires CGO for live audio capture)
go build -o avantis_utils .

# Run
./avantis_utils
```

The server listens on **http://localhost:9352**. Open it on a touchscreen or any browser.

---

## Configuration

Create a `.env` file in the project root (or alongside the binary):

```dotenv
proServer=192.168.1.140       # IP of the machine running ProPresenter
proPort=63234                 # ProPresenter network port

mixingServer=localhost        # IP of the machine running MixingStation
mixingPort=8080               # MixingStation REST port
mixingLocation=fx.rack.0.tl   # Avantis parameter path for tap tempo (default shown)
micColors="Red Blue Purple Yellow Green Pastor1 Pastor2 Orange" # space-separated mic names; each creates a <Name>_On macro
default_sound_source=Monitor of Starship/Matisse HD Audio Controller Analog Stereo # default capture device name (or substring) for tap-rate
```

> **Mic names / macros** are defined in `main.go` (`MicColors` constant). Each mic gets a macro named `<Name>_On` (e.g., `Red_On`, `Blue_On`, …). The "all off" macro is hard-coded as `Clear_Mics`.

---

## Required setup on the other apps

### MixingStation (Avantis)

1. Open **Settings → Global**.
2. **Enable API: REST / WebSocket**.
3. Note the port (default 8080) — put it in `.env` as `mixingPort`.
3. The Avantis and the machine running MixingStation must be on the same network.

### ProPresenter

1. Open **Settings → Network**.
2. Check **Enable Network**.
3. Note the port (default 63234) — put it in `.env` as `proPort`.
4. Create macros in ProPresenter matching the names used by the app:
   - One macro per mic: `Red_On`, `Blue_On`, `Purple_On`, `Yellow_On`, `Green_On`, `Pastor1_On`, `Pastor2_On`, `Orange_On`
   - One "clear all" macro: `Clear_Mics`
   These macros should trigger the appropriate slide/text to indicate a mic problem.

---

## Building from source

```bash
# Clone
git clone https://github.com/joelnorton/avantis_utils.git
cd avantis_utils

# Install Go 1.23+ (CGO enabled)
# Ubuntu/Debian: sudo apt install gcc pkg-config libasound2-dev
# Fedora:       sudo dnf install gcc pkgconfig alsa-lib-devel

# Fetch dependencies (includes github.com/gen2brain/malgo for audio capture)
go mod tidy

# Build
go build -o avantis_utils .

# Run
./avantis_utils
```

> **Non-CGO build** (no live audio capture, WAV file analysis still works):
> ```bash
> CGO_ENABLED=0 go build -o avantis_utils .
> ```
> The tap-rate button will be disabled and the device list empty.

---

## Project structure

```
.
├── main.go                 # Entry point, reads .env, starts server
├── server.go               # HTTP server, page rendering, tap-rate logic
├── send_functions.go       # ProPresenter & MixingStation HTTP calls
├── structs.go              # Shared data types
├── templates/
│   └── index.html          # Embedded UI (mic grid + tap-rate form)
├── internal/
│   ├── clickrate/          # High-level measurement API (5 s capture → median ms)
│   ├── detect/             # Peak detector + statistics (pure Go)
│   └── live/               # Audio capture (malgo + cgo stub)
└── .env                    # Your local config (git-ignored)
```

---

## Touchscreen notes

- All buttons (mic toggles + Set tap rate) are **120 × 120 px** for easy finger targeting.
- The page is a static HTML template served by Go; no JavaScript framework required.
- Works in kiosk mode: `chromium --kiosk http://localhost:9352`.

---

## License

MIT — use freely for your church/venue production.
```

Now the README is updated, the env var is documented, the code reads and uses it, and the selector persistence works. We should also verify that the selector retains the last selected option after setting the rate. Let's do a quick test:

We'll start server, open page, select device 2 (Starship...), set tap rate, see if selector stays on 2. Let's test quickly.
</think>
<tool_call>
<function=shell>
<parameter=command>
go build -o /tmp/avantis_utils . && (/tmp/avantis_utils >/tmp/avantis.log 2>&1 & echo $! > /tmp/avantis.pid); sleep 1; curl -s http://localhost:9352/ > /tmp/page7.html; echo "=== page ==="; cat /tmp/page7.html | grep -A12 'tap-rate-device' | head -15; echo "=== end ==="; kill $(cat /tmp/avantis.pid) 2>/dev/null; true