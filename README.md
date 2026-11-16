# led-clock

A clock for the two LED matrix input modules on the Framework Laptop 16.

- **Left matrix:** the time in 24-hour format, hours stacked above minutes.
- **Right matrix:** fills up over each minute, LED by LED, each LED fading
  in. Every LED is fully lit by the end of the minute. How it fills is set
  by `-fill` (see [Fill modes](#fill-modes)).
- **On the minute:** the seconds display empties by playing its fill
  backwards, sped up to last `-roll`, while each clock digit that changed
  scrambles into random noise and resolves into its new value.
- **Compact mode (`-compact left` or `-compact right`):** a single matrix
  shows both: the time flush against its left edge and the seconds filling
  its three rightmost columns.

![The clock digits at 01:23, 04:56, 17:08 and 20:39](docs/preview.png)

It runs until it receives SIGINT, SIGTERM or SIGHUP, then blanks the
displays and exits. Between refreshes it sleeps (every 50ms normally, 16ms
during the minute-change animation), and frames that haven't changed aren't
resent, so it uses well under 1% CPU.

## Building

Requires Go 1.27 or later (see `go.mod`).

```
make
```

## Running

```
./led-clock [flags]
```

| Flag | Default | Description |
|---|---|---|
| `-left` | auto | Serial device of the left matrix; not allowed with `-compact right` |
| `-right` | auto | Serial device of the right matrix; not allowed with `-compact left` |
| `-compact` | off | `left` or `right`: use just that matrix for both the time and the seconds. A bare `-compact` means `left` |
| `-brightness` | `0` | LED brightness 1-255; 0 leaves it unchanged. The daytime brightness when dimming is on |
| `-night-brightness` | `0` | LED brightness 1-255 at night; 0 disables dimming. Requires `-brightness` |
| `-dawn` | `07:00` | Time of day to start brightening to `-brightness` |
| `-dusk` | `21:00` | Time of day to start dimming to `-night-brightness` |
| `-transition` | `1h` | How long brightening and dimming take |
| `-fill` | `slideup` | How the seconds display fills, as `mode[,parameters]`: `slideup`, `slidedown`, `random`, `creepup`, `creepdown`, `spiral`, `disco`, `topo[,N]`, `infect[,N]` or `binary` (see [Fill modes](#fill-modes)) |
| `-fade` | `1s` | How long each seconds LED takes to fade in (not used by `disco`, `topo` or `infect`) |
| `-roll` | `1s` | Length of the minute-change animations; 0 disables them |
| `-interval` | `50ms` | Sleep between refreshes |
| `-roll-interval` | `16ms` | Sleep between refreshes during the minute-change animation |

Without `-left` and `-right`, the matrices are found by USB ID (`32ac:0020`)
and sorted by device path; the second becomes the left matrix and the first
the right. If they come out on the wrong sides, pass both paths explicitly,
e.g. `-left /dev/ttyACM0 -right /dev/ttyACM1`.

### Compact mode

With `-compact left` or `-compact right` (also `-compact=right`; a bare
`-compact` means `left`), only that one matrix is used. The digits move to columns 0-4,
column 5 stays dark, and columns 6-8 fill up over the minute (102 LEDs,
three per row) with the same `-fill` mode, fade and minute-change animation
as the two-matrix layout. The matrix is `-left` or `-right` if given (the
one matching the side); otherwise it's auto-detected, picking the same
matrix the two-matrix layout would use for that side when two are
attached. With only one attached it's used whichever side is asked for, as
there's no telling which side it's on.

### Fill modes

`-fill` takes a mode's name, then any parameters it has after commas,
e.g. `-fill topo,2`. Only `topo` and `infect` take one.

| `-fill` | Filling up | Emptying on the minute |
|---|---|---|
| `slideup` | From the bottom, row by row, left to right | Rolls down from the top |
| `slidedown` | From the top, row by row, left to right | Rolls up from the bottom |
| `random` | Random LEDs, in an order that changes every minute | The LEDs go out in the reverse of the order they came on |
| `creepup` | Each column grows from the bottom on its own, with spells of fast and slow growth | Each column shrinks back down, replaying its spells in reverse |
| `creepdown` | Like `creepup` upside down: each column grows from the top | Each column shrinks back up |
| `spiral` | Clockwise round the edge from the top left, winding inwards to the middle | Unwinds from the middle back out to the edge |
| `disco` | Random LEDs flash on and off, a new pattern about every 200ms; the share lit matches the share of the minute gone, to within one pattern's worth (about one LED) | The share lit races back down to none, the patterns flickering by |
| `topo[,N]` | Random pixels fade in N at a time (default 5), each pulling its neighbours part way up with it, so the light spreads in smooth, growing patches | Rewinds, the patches shrinking away |
| `infect[,N]` | N random seeds (default 3) fade in at the start of the minute, and the light spreads out from them, each LED brightening in proportion to how bright its neighbours are | Rewinds, the light drawing back into the seeds |
| `binary` | Boxes growing from the top left, each about doubling the area before it, count up in binary: box N is lit while bit N of the count is set | Counts back down to zero |

In `creepup` (and `creepdown`), each column's speed follows a squared-off sine wave with a
random period (12-30s) and phase, so it lingers at fast or slow and eases
between them, ranging from 0.2x to 1.8x its average. A guidance law steers
each column so it reaches the far end exactly at the end of the minute,
however its fast and slow spells fall: at every moment its speed is scaled
by the distance left to climb over the wobble left to come.

In `topo`, every pixel is picked once a minute, in a random order, and
N picks fade in at once (`-fill topo,2` for 2). A pick takes its own pixel to full and
pulls the pixels around it part of the way, about 30% for direct
neighbours, 18% for diagonals and 7% two pixels away. Pulls compound on
whatever darkness is left: a pixel pulled to 20%, then pulled 25% of the
rest, sits at 40%, and when it's picked itself it fades from there to full.
Since early picks light many dark neighbours while late ones only top up,
the picks are paced so that the average brightness always matches the
share of the minute gone.

In `binary`, the display is split into boxes growing from the top left,
each about doubling the area covered so far: the corner LED, the one to its
right, the two below them, the 2x2 to the right of those, and so on, each
box widening the covered rectangle when it's no wider than it is tall and
lengthening it otherwise. When a box reaches an edge, it also takes any
sliver it would leave there that's no bigger than itself, so on the full
matrix box 6 takes the last 5 columns rather than leaving 1, and the boxes
after it are full-width strips, the last taking the rows left over:

```
1 2 4 4 6 6 6 6 6
3 3 4 4 6 6 6 6 6
5 5 5 5 6 6 6 6 6
5 5 5 5 6 6 6 6 6
7 7 7 7 7 7 7 7 7   x4 rows
8 8 8 8 8 8 8 8 8   x8 rows
9 9 9 9 9 9 9 9 9   x18 rows
```

That's nine boxes of 1, 1, 2, 4, 8, 20, 36, 72 and 162 LEDs. The compact
layout's three columns make seven: 1, 2 (the rest of the top row), then
strips of 3, 6, 12, 24 and 54.

Over the minute the count goes from 0 to 2^n-1 (n being the number of
boxes), stepping every minute/(2^n-1): about 117ms with nine boxes, 472ms
with seven. Box 1 is the lowest bit, so it lights alone first, then box 2,
then both, then box 3, and at the end of the minute every box is lit. The
boxes switch fully on and off, ignoring `-fade`.

In `infect`, the minute is simulated in 50ms steps. Each step works out how
much light is still needed for every LED to be full at the end of the
minute, divides it by the steps left, and hands that out: each LED gets a
share in proportion to the light around it (direct neighbours counting
fully, diagonals half) times the room it has left to brighten, and the
seeds get an extra share of their own to start them off. No LED brightens
by more than 10% a step, and light an LED can't take goes to the others,
so the light spreads steadily outwards from the seeds while the average
brightness tracks the minute exactly.

### Dimming at night

With `-night-brightness`, the brightness follows the local time of day: it
starts ramping from `-brightness` down to `-night-brightness` at `-dusk`,
and back up at `-dawn`, each ramp taking `-transition`. For example,
`-brightness 120 -night-brightness 15 -dusk 22:00 -dawn 06:30` is fully dim
from 23:00 and fully bright from 07:30.

### Permissions

The matrices appear as `/dev/ttyACM*`, owned by the `dialout` group. Either
add yourself to `dialout`, or install Framework's udev rule
([`50-framework-inputmodule.rules`](https://github.com/FrameworkComputer/inputmodule-rs))
to give the logged-in user access.

## Running as a service

`led-clock.service` runs the clock at boot as an unprivileged, sandboxed
user in the `dialout` group, restarting it if it exits (for example when
the matrices aren't ready yet).

```
make
sudo make install-service
```

This installs the binary to `/usr/local/bin` (change with `PREFIX=` or
`BINDIR=`) and the unit to `/etc/systemd/system`, then enables and starts
it. Edit the `ExecStart` line in `led-clock.service` to change the flags.

```
journalctl -u led-clock -f    # logs
sudo make uninstall           # stop, disable and remove
```

Stop any manually started `led-clock` first; only one process can drive the
matrices at a time.

## Make targets

| Target | Description |
|---|---|
| `build` (default) | Build `led-clock` |
| `run` | Build and run in the foreground |
| `test`, `vet`, `fmt` | Go checks and formatting |
| `preview` | Render the clock digits to `docs/preview.png` |
| `install` | Install the binary |
| `install-service` | Install the binary and unit, enable and start the service |
| `uninstall` | Remove the service and binary |
| `clean` | Remove build outputs |

## Code

| File | Contents |
|---|---|
| `main.go` | Flags, device setup, refresh loop, signal handling |
| `compact.go` | The `-compact` flag, which works bare or with a side |
| `ledmatrix.go` | Serial protocol and device discovery |
| `schedule.go` | Time-of-day brightness schedule |
| `render.go` | Clock font and layout, seconds display, minute-change animations |
| `fill.go` | Seconds fill modes: the order and timing the LEDs come on in |
| `schedule_test.go` | Tests for the brightness schedule |
| `render_test.go` | Tests for the compact layout and fill modes |
| `compact_test.go` | Tests for parsing `-compact` |
| `preview_test.go` | Renders digits to a PNG for `make preview` |

### Changing the digits

The 5x7 digits are drawn as text in `render.go`, `#` for a lit pixel and
`.` for an unlit one. Run `make preview` to regenerate `docs/preview.png`
and see the result without the hardware.

### Protocol

Commands are written to the module's USB serial port as two magic bytes
`0x32 0xAC`, a command ID, then parameters. This program uses:

| ID | Command | Parameters |
|---|---|---|
| `0x00` | Brightness | 1 byte |
| `0x07` | StageCol | column index, then 34 greyscale bytes (top to bottom) |
| `0x08` | FlushCols | none; shows all staged columns at once |

See `commands.md` in
[inputmodule-rs](https://github.com/FrameworkComputer/inputmodule-rs) for the
full command set.

## License

Public domain; see [LICENSE](LICENSE).
