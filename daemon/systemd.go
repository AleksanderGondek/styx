package daemon

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func systemdGetFd(name string) (int, error) {
	pid, err := strconv.Atoi(os.Getenv("LISTEN_PID"))
	if err != nil || pid != os.Getpid() {
		return 0, errors.New("no fds passed")
	}
	nfds, err := strconv.Atoi(os.Getenv("LISTEN_FDS"))
	if err != nil || nfds == 0 {
		return 0, errors.New("no fds passed")
	}
	for fd := 3; fd < 3+nfds; fd++ {
		unix.CloseOnExec(fd)
	}
	for i, n := range strings.Split(os.Getenv("LISTEN_FDNAMES"), ":") {
		if n == name {
			return 3 + i, nil
		}
	}
	return 0, errors.New("name not found")
}

func systemdSaveFd(name string, fd int) {
	addr := systemdNotifyAddr()
	if addr == nil {
		return
	}
	srcname := fmt.Sprintf("/tmp/styx-notify-src-%x", rand.Int63())
	defer os.Remove(srcname)
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Net: "unixgram", Name: srcname})
	if err != nil {
		log.Println("error dialing unix socket", err)
		return
	}
	defer conn.Close()
	// set FDPOLL=0 since cachefiles uses POLLERR specially
	msg := fmt.Sprintf("FDSTORE=1\nFDNAME=%s\nFDPOLL=0\n", name)
	oob := unix.UnixRights(fd)
	if _, _, err := conn.WriteMsgUnix([]byte(msg), oob, addr); err != nil {
		log.Println("error writing to notify socket", err)
	}
}

func systemdReady() {
	addr := systemdNotifyAddr()
	if addr == nil {
		return
	}
	conn, err := net.DialUnix(addr.Net, nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("READY=1")); err != nil {
		log.Println("error writing to notify socket", err)
	}
}

func systemdNotifyAddr() *net.UnixAddr {
	if name := os.Getenv("NOTIFY_SOCKET"); name != "" {
		return &net.UnixAddr{Name: name, Net: "unixgram"}
	}
	return nil
}
