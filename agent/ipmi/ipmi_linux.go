//go:build linux && (amd64 || arm64 || arm || 386 || riscv64 || loong64 || s390x)

package ipmi

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Mirrors of the structures in linux/ipmi.h. Go lays these out like C on the
// architectures above (int is the size of C long), and the ioctl numbers below
// are derived from their sizes.

type ipmiSystemInterfaceAddr struct {
	addrType int32
	channel  int16
	lun      uint8
	_        uint8
}

type ipmiMsg struct {
	netFn   uint8
	cmd     uint8
	dataLen uint16
	data    unsafe.Pointer
}

type ipmiReq struct {
	addr    unsafe.Pointer
	addrLen uint32
	msgID   int
	msg     ipmiMsg
}

type ipmiRecv struct {
	recvType int32
	addr     unsafe.Pointer
	addrLen  uint32
	msgID    int
	msg      ipmiMsg
}

const (
	ipmiSystemInterfaceAddrType = 0x0c
	ipmiBMCChannel              = 0x0f
	ipmiResponseRecvType        = 1
	ipmiMaxMsgLength            = 272

	iocWrite = 1
	iocRead  = 2
)

func ioc(dir, nr, size uintptr) uintptr {
	return dir<<30 | size<<16 | 'i'<<8 | nr
}

var (
	ipmictlReceiveMsgTrunc = ioc(iocRead|iocWrite, 11, unsafe.Sizeof(ipmiRecv{}))
	ipmictlSendCommand     = ioc(iocRead, 13, unsafe.Sizeof(ipmiReq{}))
)

// DevicePaths are the character devices tried, in order, by Open.
var DevicePaths = []string{"/dev/ipmi0", "/dev/ipmi/0", "/dev/ipmidev/0"}

// commandTimeout bounds each request. BMCs normally answer in milliseconds.
const commandTimeout = 3 * time.Second

type devTransport struct {
	f     *os.File
	msgID int
}

// Open opens the first available IPMI device and returns a collector for it.
func Open() (*Collector, error) {
	var errs []error
	for _, path := range DevicePaths {
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err == nil {
			return newCollector(&devTransport{f: f}), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil, fmt.Errorf("no IPMI device found (is the ipmi_devintf module loaded?): %w", os.ErrNotExist)
	}
	return nil, errors.Join(errs...)
}

func (d *devTransport) close() error {
	return d.f.Close()
}

// allowedCommands lists the only requests sent to the BMC. The device grants
// full BMC control (power, users, fan policy) to anyone who can open it, so
// refusing everything else keeps the agent strictly read-only.
var allowedCommands = map[[2]uint8]bool{
	{netFnStorage, cmdReserveSDRRepo}:  true,
	{netFnStorage, cmdGetSDR}:          true,
	{netFnSensor, cmdGetSensorReading}: true,
}

// errCommandNotAllowed is returned for requests outside allowedCommands.
var errCommandNotAllowed = errors.New("ipmi: command not allowed")

func (d *devTransport) send(netFn, cmd, lun uint8, data []byte) ([]byte, error) {
	if !allowedCommands[[2]uint8{netFn, cmd}] {
		return nil, fmt.Errorf("%w: netfn 0x%02x cmd 0x%02x", errCommandNotAllowed, netFn, cmd)
	}
	fd := int(d.f.Fd())
	d.msgID++
	msgID := d.msgID

	addr := ipmiSystemInterfaceAddr{addrType: ipmiSystemInterfaceAddrType, channel: ipmiBMCChannel, lun: lun}
	req := ipmiReq{
		addr:    unsafe.Pointer(&addr),
		addrLen: uint32(unsafe.Sizeof(addr)),
		msgID:   msgID,
		msg:     ipmiMsg{netFn: netFn, cmd: cmd, dataLen: uint16(len(data))},
	}
	if len(data) > 0 {
		req.msg.data = unsafe.Pointer(&data[0])
	}
	if err := ioctl(fd, ipmictlSendCommand, unsafe.Pointer(&req)); err != nil {
		return nil, fmt.Errorf("ipmi send: %w", err)
	}
	runtime.KeepAlive(data)
	runtime.KeepAlive(&addr)

	deadline := time.Now().Add(commandTimeout)
	buf := make([]byte, ipmiMaxMsgLength)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf("ipmi netfn 0x%02x cmd 0x%02x: timed out", netFn, cmd)
		}
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, int(remaining.Milliseconds())+1)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("ipmi poll: %w", err)
		}
		if n == 0 {
			continue
		}

		var recvAddr ipmiSystemInterfaceAddr
		recv := ipmiRecv{
			addr:    unsafe.Pointer(&recvAddr),
			addrLen: uint32(unsafe.Sizeof(recvAddr)),
			msg:     ipmiMsg{data: unsafe.Pointer(&buf[0]), dataLen: uint16(len(buf))},
		}
		err = ioctl(fd, ipmictlReceiveMsgTrunc, unsafe.Pointer(&recv))
		runtime.KeepAlive(buf)
		runtime.KeepAlive(&recvAddr)
		if err != nil && !errors.Is(err, unix.EMSGSIZE) {
			return nil, fmt.Errorf("ipmi receive: %w", err)
		}
		// Discard late responses to earlier requests that timed out, and events.
		if recv.recvType != ipmiResponseRecvType || recv.msgID != msgID {
			continue
		}
		resp := buf[:min(int(recv.msg.dataLen), len(buf))]
		if len(resp) == 0 {
			return nil, errors.New("ipmi: empty response")
		}
		if resp[0] != ccOK {
			return nil, &CompletionCodeError{NetFn: netFn, Cmd: cmd, Code: resp[0]}
		}
		return append([]byte(nil), resp[1:]...), nil
	}
}

func ioctl(fd int, req uintptr, arg unsafe.Pointer) error {
	for {
		_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
		if errno == unix.EINTR {
			continue
		}
		if errno != 0 {
			return errno
		}
		return nil
	}
}
