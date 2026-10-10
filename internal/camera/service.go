package camera

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Device struct {
	DeviceID string `json:"deviceId"`
	Label    string `json:"label"`
}
type Devices struct {
	Native bool     `json:"native"`
	Items  []Device `json:"devices"`
	Error  string   `json:"error"`
}
type Frame struct {
	Sequence uint64 `json:"sequence"`
	Image    string `json:"image"`
	Error    string `json:"error"`
}
type session struct {
	mu      sync.Mutex
	id      string
	cmd     *exec.Cmd
	frame   Frame
	done    chan struct{}
	stopped bool
}
type Service struct {
	mu          sync.Mutex
	closing     bool
	current     *session
	closed      map[string]bool
	closedOrder []string
	devices     func() Devices
	command     func(string) (*exec.Cmd, error)
}

var configureProcess = func(*exec.Cmd) {}

func New() *Service {
	return &Service{closed: map[string]bool{}, devices: List, command: captureCommand}
}
func List() Devices {
	result := Devices{Native: runtime.GOOS == "linux", Items: []Device{}}
	if !result.Native {
		return result
	}
	if _, err := exec.LookPath("gst-launch-1.0"); err != nil {
		result.Error = "ระบบกล้องต้องใช้ gst-launch-1.0 กรุณาติดตั้ง GStreamer tools"
		return result
	}
	paths, _ := filepath.Glob("/sys/class/video4linux/video*/name")
	sort.Slice(paths, func(i, j int) bool {
		left, _ := strconv.Atoi(strings.TrimPrefix(filepath.Base(filepath.Dir(paths[i])), "video"))
		right, _ := strconv.Atoi(strings.TrimPrefix(filepath.Base(filepath.Dir(paths[j])), "video"))
		return left < right
	})
	for _, path := range paths {
		index, err := os.ReadFile(filepath.Join(filepath.Dir(path), "index"))
		if err != nil || strings.TrimSpace(string(index)) != "0" {
			continue
		}
		name, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		device := filepath.Join("/dev", filepath.Base(filepath.Dir(path)))
		if info, err := os.Stat(device); err != nil || info.Mode()&os.ModeCharDevice == 0 {
			continue
		}
		result.Items = append(result.Items, Device{DeviceID: device, Label: strings.TrimSpace(string(name))})
	}
	return result
}
func captureCommand(device string) (*exec.Cmd, error) {
	executable, err := exec.LookPath("gst-launch-1.0")
	if err != nil {
		return nil, err
	}
	return exec.Command(executable, "-q", "v4l2src", "device="+device, "do-timestamp=true", "!",
		"video/x-raw,width=640,height=480;image/jpeg,width=640,height=480", "!", "decodebin", "!",
		"videoconvert", "!", "videorate", "drop-only=true", "!", "video/x-raw,framerate=12/1", "!",
		"jpegenc", "quality=75", "!", "fdsink", "fd=1", "sync=false"), nil
}
func (s *Service) Devices() Devices { return s.devices() }
func (s *Service) Open(id, device string) (string, error) {
	if id == "" || len(id) > 100 {
		return "", errors.New("Invalid camera session")
	}
	available := s.devices()
	if available.Error != "" {
		return "", errors.New(available.Error)
	}
	if !available.Native {
		return "", errors.New("Native camera is unavailable")
	}
	if device != "" && !containsDevice(available.Items, device) {
		return "", errors.New("ไม่พบ webcam ที่เลือก กรุณาเลือกกล้องใหม่")
	}
	if device == "" {
		if len(available.Items) == 0 {
			return "", errors.New("ไม่พบ webcam กรุณาเชื่อมต่อกล้องแล้วลองใหม่")
		}
		device = available.Items[0].DeviceID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return "", errors.New("Camera service is closed")
	}
	if s.closed[id] {
		return "", errors.New("Camera session was cancelled")
	}
	if s.current != nil {
		s.current.stop()
		select {
		case <-s.current.done:
		case <-time.After(2 * time.Second):
			return "", errors.New("กล้องเดิมยังไม่ปิด กรุณาลองใหม่")
		}
		s.current = nil
	}
	cmd, err := s.command(device)
	if err != nil {
		return "", err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	diagnostic := &limitedOutput{}
	cmd.Stderr = diagnostic
	configureProcess(cmd)
	if err = cmd.Start(); err != nil {
		_ = output.Close()
		return "", err
	}
	active := &session{id: id, cmd: cmd, done: make(chan struct{})}
	s.current = active
	go active.read(output, diagnostic)
	return device, nil
}
func containsDevice(devices []Device, id string) bool {
	for _, device := range devices {
		if device.DeviceID == id {
			return true
		}
	}
	return false
}
func (s *Service) Read(id string, after uint64) (Frame, error) {
	s.mu.Lock()
	active := s.current
	s.mu.Unlock()
	if active == nil || active.id != id {
		return Frame{}, errors.New("Camera session is closed")
	}
	active.mu.Lock()
	defer active.mu.Unlock()
	result := active.frame
	if result.Sequence <= after {
		result.Image = ""
	}
	return result, nil
}
func (s *Service) Close(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed[id] {
		s.closed[id] = true
		s.closedOrder = append(s.closedOrder, id)
		if len(s.closedOrder) > 128 {
			delete(s.closed, s.closedOrder[0])
			s.closedOrder = s.closedOrder[1:]
		}
	}
	if s.current != nil && s.current.id == id {
		s.current.stop()
	}
}
func (s *Service) CloseAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closing = true
	if s.current != nil {
		s.current.stop()
	}
}
func (s *session) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	_ = s.cmd.Process.Kill()
}
func (s *session) read(output io.ReadCloser, diagnostic *limitedOutput) {
	defer close(s.done)
	defer output.Close()
	pending := make([]byte, 0, 128*1024)
	buffer := make([]byte, 32*1024)
	for {
		n, err := output.Read(buffer)
		pending = append(pending, buffer[:n]...)
		for {
			start := bytes.Index(pending, []byte{0xff, 0xd8})
			if start < 0 {
				if len(pending) > 1 {
					pending = pending[len(pending)-1:]
				}
				break
			}
			pending = pending[start:]
			end := bytes.Index(pending[2:], []byte{0xff, 0xd9})
			if end < 0 {
				break
			}
			end += 4
			image := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(pending[:end])
			s.mu.Lock()
			s.frame.Sequence++
			s.frame.Image = image
			s.mu.Unlock()
			pending = pending[end:]
		}
		if len(pending) > 1024*1024 {
			_ = s.cmd.Process.Kill()
			break
		}
		if err != nil {
			break
		}
	}
	err := s.cmd.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.stopped {
		message := strings.TrimSpace(diagnostic.String())
		if message == "" {
			message = "webcam หยุดส่งภาพ กรุณาลองเปิดกล้องอีกครั้ง"
		}
		if err != nil {
			message = fmt.Sprintf("เปิด webcam ไม่สำเร็จ: %s", message)
		}
		s.frame.Error = message
	}
}

type limitedOutput struct {
	mu   sync.Mutex
	data []byte
}

func (b *limitedOutput) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, data...)
	if len(b.data) > 8192 {
		b.data = append([]byte(nil), b.data[len(b.data)-8192:]...)
	}
	return len(data), nil
}
func (b *limitedOutput) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.data) }
