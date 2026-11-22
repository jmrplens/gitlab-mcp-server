// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build linux

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// How long the server may outlive a SIGTERM to the npx process that started
// it, and how long npx may take to install nothing and start the server. The
// launcher checks its parent once a second and the server's own shutdown
// takes well under one, so ten seconds fails only a server nothing stopped;
// a cold runner makes npx slower than it looks.
const (
	npmStopDeadline  = 10 * time.Second
	npmStartDeadline = 30 * time.Second
)

// stopUnderNpx starts the installed package the way a client configured
// with npx starts it, twice, in stdio mode with its stdin held open and with
// --http on a loopback port, sends SIGTERM to the npx process alone, as a
// supervisor does, and requires the server to be gone within npmStopDeadline
// (in HTTP mode its port closed too). npm hands the signal to the sh -c it
// runs the launcher through, the shell dies, and only the launcher's own
// parent watch can tell the server; up to 3.1.0 the launcher had none, and an
// HTTP server started this way kept its port and answered /health after npx
// was gone.
func (n *npmValidation) stopUnderNpx(project string, host releasePlatform) {
	server, err := filepath.EvalSymlinks(filepath.Join(project, "node_modules", npmScope, "gitlab-mcp-server-"+host.npmKey, binaryName))
	if err != nil {
		n.failf("stop: the installed binary cannot be found: %v", err)
		return
	}
	for _, mode := range []string{"stdio", "http"} {
		n.stopOnce(mode, project, server)
	}
}

// procStat reads the state and parent of a process from /proc, or false
// when there is no such process. The command name sits in parentheses and
// may hold spaces and parentheses itself, so the fields are read after the
// last closing one.
func procStat(pid int) (state string, ppid int, ok bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat") //#nosec G304 -- a process's stat file
	if err != nil {
		return "", 0, false
	}
	text := string(data)
	fields := strings.Fields(text[strings.LastIndex(text, ")")+1:])
	if len(fields) < 2 {
		return "", 0, false
	}
	ppid, err = strconv.Atoi(fields[1])
	return fields[0], ppid, err == nil
}

// alive is false for a process that is gone and for one that has exited and
// waits only to be reaped, which is all an orphan may be left as in a
// container whose first process does not reap.
func alive(pid int) bool {
	state, _, ok := procStat(pid)
	return ok && state != "Z"
}

// serverUnder finds the process running exe whose chain of parents reaches
// ancestor, or 0. Matching on the executable is what tells the binary this
// project installed from one npx might have fetched from the registry.
func serverUnder(ancestor int, exe string) int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		pid, atoiErr := strconv.Atoi(entry.Name())
		if atoiErr != nil {
			continue
		}
		if target, linkErr := os.Readlink("/proc/" + entry.Name() + "/exe"); linkErr != nil || target != exe {
			continue
		}
		for at, hops := pid, 0; at > 1 && hops < 16; hops++ {
			_, ppid, ok := procStat(at)
			if !ok {
				break
			}
			if ppid == ancestor {
				return pid
			}
			at = ppid
		}
	}
	return 0
}

// until polls cond every interval until it holds or deadline passes, and
// reports the time it took or false.
func until(cond func() bool, deadline, interval time.Duration) (time.Duration, bool) {
	start := time.Now()
	for time.Since(start) < deadline {
		if cond() {
			return time.Since(start), true
		}
		time.Sleep(interval)
	}
	return 0, false
}

// freePort is a loopback port nothing listens on now.
func freePort() (int, error) {
	var config net.ListenConfig
	listener, err := config.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return 0, fmt.Errorf("a loopback listener has the address %v", listener.Addr())
	}
	return address.Port, listener.Close()
}

// healthy reports whether /health on port answers 200. The connection is
// closed with the response, so the check leaves nothing idle behind for the
// shutdown to wait on.
func healthy(port int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+"/health", http.NoBody)
	if err != nil {
		return false
	}
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Do(req) //#nosec G704 -- the loopback port the server under validation listens on
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// refuses reports whether nothing accepts a connection on port.
func refuses(port int) bool {
	dialer := net.Dialer{Timeout: time.Second}
	conn, err := dialer.DialContext(context.Background(), "tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return true
	}
	_ = conn.Close()
	return false
}

// stopOnce runs the stop check in one transport. --offline keeps npx from
// reaching the registry, where a published version of the same name would
// otherwise be a candidate.
//
// The server's stdio is a FIFO and two files rather than pipes: npm shuts
// its stdio down as it exits, and since every process down the chain shares
// it, the server would then read an end of input and die writing to a closed
// stderr. That ends the server whatever the launcher does, so a check over
// pipes passes a launcher that forwards nothing. A client holding ordinary
// pipes, or a supervisor logging to a file, sees no such end, and that is the
// case the check stands for. The FIFO is opened for reading and writing, so
// the server never sees the end of its input while the check runs.
func (n *npmValidation) stopOnce(mode, project, exe string) {
	label := "stop (" + mode + ")"
	run, err := n.prepareStop(mode)
	if err != nil {
		n.failf("%s: %v", label, err)
		return
	}
	defer run.close()
	npx, exited, err := run.start(n.npx, project)
	if err != nil {
		n.failf("%s: npx could not be started: %v", label, err)
		return
	}
	server, launcher := 0, 0
	defer func() {
		// Whatever a failed check left running is ended here, so the
		// validator never leaves a server holding a port or a token.
		for _, pid := range []int{server, launcher} {
			if pid > 0 && alive(pid) {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
		select {
		case <-exited:
		default:
			_ = npx.Process.Kill()
			<-exited
		}
	}()
	_, ready := until(func() bool {
		if server == 0 {
			server = serverUnder(npx.Process.Pid, exe)
		}
		return server != 0 && run.answered(n.version)
	}, npmStartDeadline, 200*time.Millisecond)
	if !ready {
		state := "never answered"
		if server == 0 {
			state = "never started"
		}
		n.failf("%s: through npx the server %s within %s; npx stderr: %s", label, state, npmStartDeadline, run.stderrTail())
		return
	}
	_, launcher, _ = procStat(server)
	_ = npx.Process.Signal(syscall.SIGTERM)
	took, gone := until(func() bool { return !alive(server) }, npmStopDeadline, 100*time.Millisecond)
	if !gone {
		state, _, _ := procStat(server)
		n.failf("%s: the server outlived a SIGTERM to npx by %s (pid %d, state %s)", label, npmStopDeadline, server, state)
		return
	}
	if mode == "http" {
		n.check(refuses(run.port), "%s: port %d still accepts connections after the server exited", label, run.port)
	}
	fmt.Fprintf(n.stdout, "%s: SIGTERM to npx ended the server in %s\n", label, took.Round(time.Millisecond))
}

// stopRun is one transport's run of the stop check: the arguments npx is
// given, the port in HTTP mode, the FIFO the server reads in stdio mode,
// and the files its output goes to.
type stopRun struct {
	mode, version    string
	args             []string
	port             int
	stdin            *os.File
	outPath, errPath string
}

// prepareStop lays out a run of the stop check in mode.
func (n *npmValidation) prepareStop(mode string) (*stopRun, error) {
	dir, err := os.MkdirTemp(n.work, "stop-"+mode+"-")
	if err != nil {
		return nil, err
	}
	run := &stopRun{
		mode: mode, version: n.version,
		args:    []string{"--offline", "--yes", npmMainName + "@" + n.version},
		outPath: filepath.Join(dir, "stdout"), errPath: filepath.Join(dir, "stderr"),
	}
	if mode == "http" {
		if run.port, err = freePort(); err != nil {
			return nil, err
		}
		run.args = append(run.args, "--http", "--http-addr", "127.0.0.1:"+strconv.Itoa(run.port), "--allow-any-gitlab-url")
		return run, nil
	}
	fifo := filepath.Join(dir, "stdin")
	if fifoErr := syscall.Mkfifo(fifo, 0o600); fifoErr != nil {
		return nil, fifoErr
	}
	if run.stdin, err = os.OpenFile(fifo, os.O_RDWR, 0); err != nil { //#nosec G304 -- the FIFO this check made
		return nil, err
	}
	return run, nil
}

// close releases the FIFO.
func (r *stopRun) close() {
	if r.stdin != nil {
		_ = r.stdin.Close()
	}
}

// start starts npx with the server's output in the run's files, sends the
// initialize request in stdio mode, and returns npx and a channel closed
// once it has been waited for.
func (r *stopRun) start(npxPath, project string) (*exec.Cmd, <-chan struct{}, error) {
	out, err := os.Create(r.outPath) //#nosec G304 -- a file in the check's own directory
	if err != nil {
		return nil, nil, err
	}
	errFile, err := os.Create(r.errPath) //#nosec G304 -- a file in the check's own directory
	if err != nil {
		_ = out.Close()
		return nil, nil, err
	}
	npx := exec.CommandContext(context.Background(), npxPath, r.args...) //#nosec G204 -- npx over the package this validator installed
	npx.Dir = project
	npx.Env = append(os.Environ(), "GITLAB_URL=https://gitlab.com", "GITLAB_TOKEN=glpat-validate-invalid")
	npx.Stdin, npx.Stdout, npx.Stderr = r.stdin, out, errFile
	err = npx.Start()
	_, _ = out.Close(), errFile.Close()
	if err != nil {
		return nil, nil, err
	}
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		_ = npx.Wait()
	}()
	if r.stdin != nil {
		_, _ = r.stdin.WriteString(handshakeRequest)
	}
	return npx, exited, nil
}

// answered reports whether the server is up: /health answering in HTTP
// mode, the initialize answer naming the version in stdio mode.
func (r *stopRun) answered(version string) bool {
	if r.mode == "http" {
		return healthy(r.port)
	}
	printed, _ := os.ReadFile(r.outPath) //#nosec G304 -- the server's stdout, in the check's own directory
	return strings.Contains(string(printed), `"id":1`) && strings.Contains(string(printed), `"version":"`+version+`"`)
}

// stderrTail is the end of what npx wrote on stderr.
func (r *stopRun) stderrTail() string {
	tail, _ := os.ReadFile(r.errPath) //#nosec G304 -- npx's stderr, in the check's own directory
	return strings.TrimSpace(string(tail[max(0, len(tail)-2000):]))
}
