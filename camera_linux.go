package main

/*
#cgo pkg-config: gtk+-3.0
#cgo !webkit2_41 pkg-config: webkit2gtk-4.0
#cgo webkit2_41 pkg-config: webkit2gtk-4.1
void praewa_camera_install(void);
void praewa_camera_access(int enabled);
*/
import "C"

func prepareCamera() {
	C.praewa_camera_install()
}

func (a *App) CameraAccess(enabled bool) {
	value := C.int(0)
	if enabled {
		value = 1
	}
	C.praewa_camera_access(value)
}
