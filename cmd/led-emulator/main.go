// Command led-emulator emulates Framework 16 LED matrix modules, so
// led-clock can be run and watched without the hardware.
//
// Each emulated module is a pseudo-terminal that decodes the serial
// protocol as the module's firmware does. The panels are shown, lit as they
// would be, on a web page.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8034", "address to serve the web view on")
	names := flag.String("panels", "left,right", "comma-separated names of the panels to emulate, shown left to right; one name emulates a single module, for -compact")
	dir := flag.String("dir", filepath.Join(os.TempDir(), "led-emulator"), "directory to create a symlink to each panel's port in, named after the panel (empty to skip)")
	brightness := flag.Uint("brightness", 255, "brightness (0-255) the panels start at, until a Brightness command sets it")
	hint := flag.Bool("hint", true, "print the panels' ports and the led-clock command to run against them (led-clock -emulator turns this off)")
	flag.Parse()
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unexpected argument %q\n", flag.Arg(0))
		flag.Usage()
		os.Exit(2)
	}
	if *brightness > 255 {
		log.Fatal("-brightness must be 0-255")
	}

	panelNames := strings.Split(*names, ",")
	seen := map[string]bool{}
	for _, n := range panelNames {
		if n == "" || strings.ContainsAny(n, `/\`) || n == "." || n == ".." {
			log.Fatalf("-panels: bad panel name %q", n)
		}
		if seen[n] {
			log.Fatalf("-panels: %q given twice", n)
		}
		seen[n] = true
	}

	hub := NewHub()
	var panels []*Panel
	var links []string
	defer func() {
		for _, l := range links {
			os.Remove(l)
		}
	}()
	for _, name := range panelNames {
		pty, err := openPTY()
		if err != nil {
			log.Fatalf("creating pty: %v", err)
		}
		path := pty.Path
		if *dir != "" {
			link, err := makeLink(*dir, name, path)
			if err != nil {
				log.Fatal(err)
			}
			links = append(links, link)
			path = link
		}
		p := NewPanel(name, path, byte(*brightness), hub)
		panels = append(panels, p)
		go readPort(p, pty)
	}

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		log.Fatal(http.Serve(ln, newServer(panels, hub)))
	}()

	fmt.Printf("Emulating %d LED matrix module(s). Watch them at http://%s/\n", len(panels), ln.Addr())
	if *hint {
		fmt.Println()
		for _, p := range panels {
			fmt.Printf("  %-8s %s\n", p.Name, p.Path)
		}
		fmt.Printf("\nRun the clock against them with:\n\n  %s\n\n", clockCommand(panels))
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	<-sigs
}

// makeLink creates dir/name as a symlink to target, replacing a stale
// symlink from an earlier run but nothing else.
func makeLink(dir, name, target string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	link := filepath.Join(dir, name)
	if fi, err := os.Lstat(link); err == nil {
		if fi.Mode()&os.ModeSymlink == 0 {
			return "", fmt.Errorf("%s exists and isn't a symlink; not replacing it", link)
		}
		if err := os.Remove(link); err != nil {
			return "", err
		}
	}
	if err := os.Symlink(target, link); err != nil {
		return "", err
	}
	return link, nil
}

// clockCommand is the led-clock command line for the emulated panels: the
// compact layout on a single panel, else the first two as left and right.
func clockCommand(panels []*Panel) string {
	if len(panels) == 1 {
		return fmt.Sprintf("./led-clock -compact left -left %s", panels[0].Path)
	}
	return fmt.Sprintf("./led-clock -left %s -right %s", panels[0].Path, panels[1].Path)
}

// idleAfter is how long without commands before the port's lock is
// released, in case whatever had it open was killed.
const idleAfter = time.Second

// readPort feeds everything written to the panel's port to the panel.
func readPort(p *Panel, pty *PTY) {
	var last atomic.Int64 // UnixNano of the last read
	go func() {
		for range time.Tick(idleAfter / 4) {
			if time.Since(time.Unix(0, last.Load())) < idleAfter {
				continue
			}
			// Whatever has the port open has either gone or gone quiet
			// (the clock digits only change once a minute). If it was
			// killed, it left the port locked, and a new led-clock
			// couldn't open it; if it's still running, unlocking does no
			// harm. Either way a half-sent command won't be finished.
			p.Reset()
			if err := pty.ReleaseLock(); err != nil {
				log.Printf("%s: releasing port lock: %v", p.Name, err)
			}
		}
	}()
	buf := make([]byte, 4096)
	for {
		n, err := pty.Master.Read(buf)
		if n > 0 {
			last.Store(time.Now().UnixNano())
			p.Write(buf[:n])
		}
		if err != nil {
			log.Printf("%s: read: %v", p.Name, err)
			time.Sleep(100 * time.Millisecond)
		}
	}
}
