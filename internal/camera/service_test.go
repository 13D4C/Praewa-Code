package camera

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCameraHelperProcess(t *testing.T) {
	mode := os.Getenv("PRAEWA_CAMERA_TEST_HELPER")
	if mode == "" {
		return
	}
	if mode == "error" {
		_, _ = os.Stderr.WriteString("camera device busy")
		os.Exit(1)
	}
	if mode == "oversized" {
		_, _ = os.Stdout.Write(append([]byte{0xff, 0xd8}, bytes.Repeat([]byte{1}, 2*1024*1024)...))
		time.Sleep(time.Hour)
		return
	}
	_, _ = os.Stdout.WriteString("noise")
	for index := 0; index < 5; index++ {
		picture := image.NewRGBA(image.Rect(0, 0, 4, 4))
		picture.Set(0, 0, color.RGBA{R: uint8(index * 40), A: 255})
		var data bytes.Buffer
		_ = jpeg.Encode(&data, picture, nil)
		encoded := data.Bytes()
		for len(encoded) > 0 {
			size := min(31, len(encoded))
			_, _ = os.Stdout.Write(encoded[:size])
			encoded = encoded[size:]
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(time.Hour)
}
func testService(mode string) *Service {
	service := New()
	service.devices = func() Devices {
		return Devices{Native: true, Items: []Device{{DeviceID: "/dev/video-test", Label: "Test webcam"}}}
	}
	service.command = func(string) (*exec.Cmd, error) {
		command := exec.Command(os.Args[0], "-test.run=^TestCameraHelperProcess$")
		command.Env = append(os.Environ(), "PRAEWA_CAMERA_TEST_HELPER="+mode)
		return command, nil
	}
	return service
}
func awaitFrame(t *testing.T, service *Service, id string, accept func(Frame) bool) Frame {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		frame, err := service.Read(id, 0)
		if err != nil {
			t.Fatal(err)
		}
		if accept(frame) {
			return frame
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("camera did not produce the expected frame state")
	return Frame{}
}
func TestCameraKeepsLatestFragmentedJPEGAndStopsItsProcess(t *testing.T) {
	service := testService("frames")
	defer service.CloseAll()
	device, err := service.Open("first", "")
	if err != nil || device != "/dev/video-test" {
		t.Fatalf("Open: %s, %v", device, err)
	}
	frame := awaitFrame(t, service, "first", func(frame Frame) bool { return frame.Sequence == 5 })
	if !strings.HasPrefix(frame.Image, "data:image/jpeg;base64,") {
		t.Fatalf("Invalid preview: %s", frame.Image)
	}
	next, err := service.Read("first", frame.Sequence)
	if err != nil || next.Image != "" {
		t.Fatalf("Repeated frame was returned: %+v, %v", next, err)
	}
	process := service.current
	service.Close("first")
	select {
	case <-process.done:
	case <-time.After(2 * time.Second):
		t.Fatal("camera process was not reaped")
	}
	service.Close("first")
	if _, err := service.Open("first", ""); err == nil {
		t.Fatal("cancelled camera session reopened")
	}
}
func TestCancelledStartCannotOpenAndStaleStopCannotCloseNewCamera(t *testing.T) {
	service := testService("frames")
	defer service.CloseAll()
	if _, err := service.Open("invalid", "/tmp/other-device"); err == nil {
		t.Fatal("unlisted device was accepted")
	}
	service.Close("cancelled")
	if _, err := service.Open("cancelled", ""); err == nil {
		t.Fatal("a late start reopened a cancelled camera")
	}
	if _, err := service.Open("current", ""); err != nil {
		t.Fatal(err)
	}
	service.Close("cancelled")
	awaitFrame(t, service, "current", func(frame Frame) bool { return frame.Sequence > 0 })
	old := service.current
	if _, err := service.Open("replacement", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-old.done:
	default:
		t.Fatal("previous process remained alive after switching camera")
	}
	service.Close("current")
	awaitFrame(t, service, "replacement", func(frame Frame) bool { return frame.Sequence > 0 })
	service.CloseAll()
	if _, err := service.Open("after-shutdown", ""); err == nil {
		t.Fatal("camera reopened after shutdown")
	}
}
func TestCameraFailureAndOversizedFramesBecomeErrors(t *testing.T) {
	for _, mode := range []string{"error", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			service := testService(mode)
			defer service.CloseAll()
			if _, err := service.Open("failure", ""); err != nil {
				t.Fatal(err)
			}
			frame := awaitFrame(t, service, "failure", func(frame Frame) bool { return frame.Error != "" })
			if mode == "error" && !strings.Contains(frame.Error, "camera device busy") {
				t.Fatalf("missing device diagnostic: %s", frame.Error)
			}
		})
	}
}
