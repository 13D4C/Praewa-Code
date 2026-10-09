//go:build !linux

package main

func prepareCamera() {}

func (a *App) CameraAccess(bool) {}
