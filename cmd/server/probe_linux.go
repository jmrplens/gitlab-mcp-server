package main

// peerStdinIsNull reads a peer's file descriptor 0 from procfs, which is what
// its --transport auto read to choose HTTP or stdio.
func peerStdinIsNull(pid int32) (bool, error) {
	return stdinIsNullUnder("/proc", pid)
}
