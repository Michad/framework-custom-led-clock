package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// PTY is a pseudo-terminal standing in for a module's serial port:
// led-clock opens the slave end at Path, and what it writes comes out of
// Master.
type PTY struct {
	Master *os.File
	Path   string
	// slave is held open so that reads from Master never fail when
	// led-clock isn't running, and so that ReleaseLock can clear the lock
	// a killed led-clock leaves behind.
	slave *os.File
}

func openPTY() (*PTY, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}
	fd := int(master.Fd())
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		master.Close()
		return nil, fmt.Errorf("unlock pty: %w", err)
	}
	n, err := unix.IoctlGetUint32(fd, unix.TIOCGPTN)
	if err != nil {
		master.Close()
		return nil, fmt.Errorf("get pty number: %w", err)
	}
	path := fmt.Sprintf("/dev/pts/%d", n)
	slave, err := os.OpenFile(path, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, err
	}
	// Raw mode, as led-clock sets it, so nothing is translated even before
	// it opens the port.
	sfd := int(slave.Fd())
	t, err := unix.IoctlGetTermios(sfd, unix.TCGETS)
	if err == nil {
		t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
		t.Oflag &^= unix.OPOST
		t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
		t.Cflag &^= unix.CSIZE | unix.PARENB
		t.Cflag |= unix.CS8
		err = unix.IoctlSetTermios(sfd, unix.TCSETS, t)
	}
	if err != nil {
		master.Close()
		slave.Close()
		return nil, fmt.Errorf("set raw mode: %w", err)
	}
	return &PTY{Master: master, Path: path, slave: slave}, nil
}

// ReleaseLock clears the exclusive-use lock led-clock's serial library
// takes on the port (TIOCEXCL). It clears it again when it closes the port,
// but not if it's killed, and then the next led-clock couldn't open it.
func (p *PTY) ReleaseLock() error {
	return unix.IoctlSetInt(int(p.slave.Fd()), unix.TIOCNXCL, 0)
}
