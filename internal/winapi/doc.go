// Package winapi wraps the Win32 primitives shared by the agent and the UI:
// single-instance mutex, session-end notification, detached process
// creation, the HKCU Run entry used to start with Windows and the foreground
// permission (AllowSetForegroundWindow). All implementations live in
// *_windows.go files.
package winapi
