package main

// No test in this file may call t.Parallel: they use t.Setenv and t.Chdir,
// both of which panic in a parallel test, and TestServeShutsDownOnSIGTERM
// signals the whole process.

import (
	"bytes"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ItemizeNTNU/website/internal/config"
)

// serverEnv is every variable config.Load consults. Clearing all of them is
// what stops a contributor's own exported BASE_URL or MONGO_DB_URL from
// deciding whether a case here passes.
var serverEnv = []string{
	"NODE_ENV", "ENV",
	"PORT", "LISTEN",
	"BASE_URL",
	"FUSION_AUTH_HOST",
	"FUSION_AUTH_CLIENT_ID",
	"FUSION_AUTH_CLIENT_SECRET",
	"FUSION_AUTH_SECRET",
	"FUSION_AUTH_API_TOKEN",
	"FUSION_AUTH_ID_TOKEN_ALG",
	"FUSION_AUTH_ID_TOKEN_HMAC_SECRET",
	"MONGO_DB_URL",
	"MONGO_DB_NAME",
	"DISCORD_CLIENT_ID",
	"DISCORD_CLIENT_SECRET",
	"DISCORD_BOT_TOKEN",
	"DISCORD_SERVER_ID",
	"DISCORD_SERVER_MEMBER_ROLE_ID",
}

// withServerEnv installs env as the entire server environment and moves the
// test into an empty directory, so the ./.env that config.Load reads outside
// production is guaranteed not to exist.
func withServerEnv(t *testing.T, env map[string]string) {
	t.Helper()
	t.Chdir(t.TempDir())
	for _, key := range serverEnv {
		t.Setenv(key, "")
	}
	for key, value := range env {
		t.Setenv(key, value)
	}
}

// discardLogger keeps the server's own log output out of the test run.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// keepDefaultLogger restores the process-wide default logger afterwards. run
// calls slog.SetDefault, and a test that leaves the default pointing at its own
// handler would silently change what every later test logs.
func keepDefaultLogger(t *testing.T) {
	t.Helper()
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
}

// A server that cannot be configured must never reach the point of listening.
// Half-starting — binding the port, then failing on the first request because
// there is no identity provider — is what makes a bad rollout look healthy.
func TestRunRefusesToStartWithoutConfiguration(t *testing.T) {
	keepDefaultLogger(t)
	withServerEnv(t, nil)

	err := run(false)
	if err == nil {
		t.Fatal("run started the server with no configuration at all")
	}
	for _, want := range []string{
		"BASE_URL is required",
		"FUSION_AUTH_HOST is required",
		"FUSION_AUTH_CLIENT_ID is required",
		"FUSION_AUTH_SECRET is required",
		"MONGO_DB_URL is required",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("run's error does not mention %q, so an operator fixing the deployment has to restart to find that problem.\nfull error: %v", want, err)
		}
	}
}

// The calendar is half the reason the site exists, so a database that will not
// answer is fatal in production rather than a warning — a deployment that comes
// up with no events must not be mistaken for a healthy one.
//
// The connection string here is rejected by the driver before any socket is
// opened, which is what keeps this test off the network.
func TestRunFailsOnAnUnusableDatabaseInProduction(t *testing.T) {
	keepDefaultLogger(t)
	withServerEnv(t, map[string]string{
		"ENV":                       "production",
		"BASE_URL":                  "https://itemize.no",
		"FUSION_AUTH_HOST":          "https://auth.itemize.no",
		"FUSION_AUTH_CLIENT_ID":     "5c1b8e2a-0000-4000-8000-000000000001",
		"FUSION_AUTH_CLIENT_SECRET": "client-secret",
		"FUSION_AUTH_SECRET":        strings.Repeat("a", 32),
		// Valid enough for config.Load — it names a database — and rejected by
		// the driver's own URI parser the moment it is applied.
		"MONGO_DB_URL": "mongodb://localhost:27017/website?connectTimeoutMS=ikke-et-tall",
	})

	err := run(false)
	if err == nil {
		t.Fatal("run started in production without a database; the events page would be empty and the rollout would look successful")
	}
	if !strings.Contains(err.Error(), "MongoDB") && !strings.Contains(err.Error(), "mongo") {
		t.Errorf("run's error does not point at the database (%v); the operator has nothing to act on", err)
	}
}

// The same unreachable database is fatal in production and a warning in
// development: a contributor has to be able to work on the content pages
// without running MongoDB at all, and the warning is the only thing telling
// them why the calendar is empty.
func TestOpenEvents(t *testing.T) {
	// Rejected by the driver's URI parser, so no socket is opened either way.
	cfg := &config.Config{Mongo: config.Mongo{
		URI:      "mongodb://localhost:27017/website?connectTimeoutMS=ikke-et-tall",
		Database: "website",
	}}

	t.Run("development carries on without a database", func(t *testing.T) {
		var logged bytes.Buffer
		log := slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelDebug}))

		repo, disconnect, err := openEvents(cfg, log, true)
		if err != nil {
			t.Fatalf("a missing database stopped a development server from starting: %v", err)
		}
		if repo != nil {
			t.Error("openEvents returned a repository backed by a connection that was never established")
		}
		if disconnect == nil {
			t.Fatal("openEvents returned a nil disconnect function; run defers it unconditionally and would panic")
		}
		// Safe to call even though nothing was opened — run defers it before
		// it knows whether there is a connection.
		disconnect()

		if !strings.Contains(logged.String(), "no database") {
			t.Errorf("nothing was logged about the missing database, so an empty calendar looks like a bug in the site.\nlog: %s", logged.String())
		}
	})

	t.Run("production refuses to start", func(t *testing.T) {
		_, disconnect, err := openEvents(cfg, discardLogger(), false)
		if err == nil {
			t.Fatal("a production server started without a database; the deployment would look healthy with no events on it")
		}
		if disconnect == nil {
			t.Fatal("openEvents returned a nil disconnect function alongside its error; run defers it before checking the error")
		}
		disconnect()
	})
}

// A listen address that cannot be bound has to come back out of serve. The
// goroutine that calls ListenAndServe is the only thing that sees the error, so
// a broken hand-off here is a process that exits zero without ever serving.
func TestServeReturnsListenErrors(t *testing.T) {
	// Held open for the duration so the "address already in use" case has
	// something to collide with.
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("taking an ephemeral port failed: %v", err)
	}
	t.Cleanup(func() { _ = taken.Close() })

	tests := []struct {
		name string
		addr string
	}{
		{name: "port out of range", addr: "127.0.0.1:99999"},
		{name: "not an address at all", addr: "ikke-en-adresse"},
		{name: "address already in use", addr: taken.Addr().String()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &http.Server{Addr: tt.addr, Handler: http.NewServeMux()}
			t.Cleanup(func() { _ = srv.Close() })

			done := make(chan error, 1)
			go func() { done <- serve(srv, discardLogger()) }()

			select {
			case err := <-done:
				if err == nil {
					t.Errorf("serve(%q) returned no error; the process would exit successfully having never listened", tt.addr)
				}
			case <-time.After(30 * time.Second):
				t.Fatalf("serve(%q) never returned; a bind failure would hang the process instead of reporting it", tt.addr)
			}
		})
	}
}

// The whole point of the shutdown path is that a redeploy does not cut off a
// member mid-request: SIGTERM stops the listener and lets what is in flight
// finish. If the signal is not wired up, the container is killed after the
// orchestrator's grace period instead.
func TestServeShutsDownOnSIGTERM(t *testing.T) {
	// Keep a handler registered for the whole test. Without one, a SIGTERM
	// that lands before serve has installed its own would be delivered with
	// its default disposition and kill the test binary outright.
	guard := make(chan os.Signal, 1)
	signal.Notify(guard, syscall.SIGTERM)
	t.Cleanup(func() { signal.Stop(guard) })

	// Port zero: the kernel picks a free port, so this can never fail because
	// something else on the machine holds a fixed one.
	srv := &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()}
	t.Cleanup(func() { _ = srv.Close() })

	done := make(chan error, 1)
	go func() { done <- serve(srv, discardLogger()) }()

	// Signalling repeatedly rather than once removes the race with serve
	// installing its handler: an early SIGTERM is absorbed by the guard above
	// and simply retried. Extra signals after serve returns are harmless.
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(30 * time.Second)

	for {
		if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
			t.Fatalf("signalling the test process failed: %v", err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("serve returned %v on SIGTERM; a clean redeploy would be reported as a crash", err)
			}
			// The listener must actually be closed, not merely reported as
			// shut down — otherwise the port stays held by a process that
			// believes it has stopped.
			if err := srv.ListenAndServe(); err != http.ErrServerClosed {
				t.Errorf("the server was not left in a shut-down state (ListenAndServe returned %v)", err)
			}
			return
		case <-ticker.C:
		case <-deadline:
			t.Fatal("serve did not return after SIGTERM; the orchestrator would have to kill the container")
		}
	}
}

// The container health check runs the binary against itself, so PORT is read
// twice by two different pieces of code: config.ResolveAddr turns it into a
// listen address, healthcheck turns it into a URL. Every form the server binds
// has to be probeable, or the container is reported unhealthy — and eventually
// restarted — while it is serving perfectly well.
//
// The server here listens on 127.0.0.1, which every one of these forms reaches.
func TestHealthcheckProbesEveryAddressForm(t *testing.T) {
	port, _ := startHealthServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name string
		port string
	}{
		{name: "a bare port", port: port},
		// docker-compose and .env.example both document LISTEN, and ":3000" is
		// a perfectly ordinary thing to put in it.
		{name: "a leading colon", port: ":" + port},
		{name: "a full host:port", port: "127.0.0.1:" + port},
		// A wildcard bind is not an address to connect to; the probe has to
		// fall back to loopback rather than dial 0.0.0.0.
		{name: "a wildcard host:port", port: "0.0.0.0:" + port},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PORT", tt.port)
			t.Setenv("LISTEN", "")

			if got := healthcheck(); got != 0 {
				t.Errorf("healthcheck returned %d for PORT=%q, but the server is answering; the container would be marked unhealthy and restarted while serving normally", got, tt.port)
			}
		})
	}
}

// IPv6 is split out because the loopback interface is not guaranteed to exist
// on every build machine. The bracketed form is the one thing a naive
// "host:" + port concatenation gets wrong even when the host is right.
func TestHealthcheckProbesAnIPv6Address(t *testing.T) {
	ln, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skipf("no IPv6 loopback on this machine: %v", err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	t.Setenv("PORT", ln.Addr().String())
	t.Setenv("LISTEN", "")

	if got := healthcheck(); got != 0 {
		t.Errorf("healthcheck returned %d for PORT=%q, but the server is answering there", got, ln.Addr())
	}
}

// probeURL is what turns the bound address into something dialable. The
// wildcard cases are the ones that matter in the container: the server binds
// every interface, and the probe has to pick the one it shares with it.
func TestProbeURL(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		want    string
		wantErr bool
	}{
		{name: "a bare port from PORT=3000", addr: ":3000", want: "http://127.0.0.1:3000/healthz"},
		{name: "an explicit loopback host", addr: "127.0.0.1:3000", want: "http://127.0.0.1:3000/healthz"},
		{name: "an IPv4 wildcard probes loopback", addr: "0.0.0.0:3000", want: "http://127.0.0.1:3000/healthz"},
		{name: "an IPv6 wildcard probes IPv6 loopback", addr: "[::]:3000", want: "http://[::1]:3000/healthz"},
		{name: "an IPv6 host keeps its brackets", addr: "[::1]:3000", want: "http://[::1]:3000/healthz"},
		{name: "a named host", addr: "localhost:3000", want: "http://localhost:3000/healthz"},
		// Not reachable through config.ResolveAddr any more, but probeURL must
		// still refuse rather than build a URL that will not parse.
		{name: "no port at all", addr: "3000", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := probeURL(tt.addr)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("probeURL(%q) = %q; an address that cannot be split must be reported, not turned into a malformed URL", tt.addr, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("probeURL(%q) failed: %v; the health check cannot probe an address the server binds", tt.addr, err)
			}
			if got != tt.want {
				t.Errorf("probeURL(%q) = %q, want %q; the probe would dial the wrong place and report a serving container as unhealthy", tt.addr, got, tt.want)
			}
		})
	}
}

// An unusable PORT stops the server from starting, so -healthcheck must not
// answer 0 for it either — a probe that succeeds against a configuration the
// server refuses would mark a container healthy that can never come up.
func TestHealthcheckRejectsAnUnusablePort(t *testing.T) {
	for _, port := range []string{"0", "99999", "ikke-et-tall"} {
		t.Run(port, func(t *testing.T) {
			t.Setenv("PORT", port)
			t.Setenv("LISTEN", "")

			if got := healthcheck(); got != 1 {
				t.Errorf("healthcheck returned %d for PORT=%q, which the server itself refuses to bind", got, port)
			}
		})
	}
}

// mainArgsEnv both marks the re-executed child process and carries the command
// line main should see. Its presence is the only thing that distinguishes the
// child from an ordinary test run.
const mainArgsEnv = "ITEMIZE_TEST_MAIN_ARGS"

// TestMainHelperProcess is not a test. It is the entry point of the child
// process spawned by TestMainDispatch: main calls os.Exit, so the only way to
// observe what it does with a flag is from outside the process.
//
// os.Args is rewritten rather than passed on the child's command line because
// the testing package parses the real command line first, and would reject
// -healthcheck as an unknown flag before main ever registers it.
func TestMainHelperProcess(t *testing.T) {
	args, ok := os.LookupEnv(mainArgsEnv)
	if !ok {
		t.Skip("not the re-executed child process")
	}
	os.Args = append([]string{"website"}, strings.Fields(args)...)
	main()
}

// main is a flag switch and an exit code, and both matter operationally: the
// container health check depends on -healthcheck exiting 0 or 1, and the
// orchestrator's restart loop depends on a failed start exiting non-zero.
func TestMainDispatch(t *testing.T) {
	// A server the child can probe, and a port that is guaranteed to have
	// nothing on it once the second server is shut down.
	livePort, _ := startHealthServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	deadPort, shutdown := startHealthServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	shutdown()

	tests := []struct {
		name     string
		args     string
		env      []string
		wantExit int
		wantOut  string
	}{
		{
			name:     "-healthcheck reports a serving instance",
			args:     "-healthcheck",
			env:      []string{"PORT=" + livePort},
			wantExit: 0,
		},
		{
			name:     "-healthcheck reports an instance that is not answering",
			args:     "-healthcheck",
			env:      []string{"PORT=" + deadPort},
			wantExit: 1,
			wantOut:  "healthcheck:",
		},
		{
			// No flags: main goes on to start the server, which refuses an
			// empty environment. Exiting non-zero is what makes the
			// orchestrator retry rather than mark the deployment healthy.
			name:     "a failed start exits non-zero and says why",
			args:     "",
			wantExit: 1,
			wantOut:  "BASE_URL is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestMainHelperProcess$")
			// A deliberately minimal environment: nothing the parent happens
			// to export can configure the child by accident.
			cmd.Env = append([]string{
				mainArgsEnv + "=" + tt.args,
				"PATH=" + os.Getenv("PATH"),
				"HOME=" + os.Getenv("HOME"),
			}, tt.env...)
			// An empty directory, so the .env that config.Load reads outside
			// production cannot exist.
			cmd.Dir = t.TempDir()

			out, err := cmd.CombinedOutput()
			exit := cmd.ProcessState.ExitCode()
			if exit == -1 {
				t.Fatalf("the child process did not exit normally: %v\noutput:\n%s", err, out)
			}
			if exit != tt.wantExit {
				t.Errorf("`website %s` exited %d, want %d\noutput:\n%s", tt.args, exit, tt.wantExit, out)
			}
			if tt.wantOut != "" && !strings.Contains(string(out), tt.wantOut) {
				t.Errorf("`website %s` printed nothing about %q, so the failure is invisible in the container log\noutput:\n%s", tt.args, tt.wantOut, out)
			}
			// main always ends in os.Exit, so the testing package never gets
			// to print its verdict. Seeing it means the helper returned
			// without main taking over, and the exit code above proves
			// nothing.
			if strings.Contains(string(out), "PASS") {
				t.Errorf("the child finished as an ordinary test run rather than through main\noutput:\n%s", out)
			}
		})
	}
}
