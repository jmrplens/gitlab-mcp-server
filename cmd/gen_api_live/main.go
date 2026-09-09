package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/provenance"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// Floors below which the record is not describing a GitLab.
//
// They exist for one failure this cannot otherwise see: an introspection that
// half ran, or a boot whose classes were not all loaded, produces a smaller
// record rather than an error, and a smaller record reads downstream as GitLab
// having stopped sending things. Each is far under the figures a real instance
// gives (582 entities, 7317 fields, 2110 routes, 264 features on 19.3.1-ee) so
// an ordinary release-to-release change never trips one.
const (
	minEntities = 400
	minFields   = 5000
	minRoutes   = 1500
	minFeatures = 150
)

// dumped is the introspection as it leaves the container, before this command
// wraps it with provenance. It is a type of its own so the digest is taken
// over exactly what the script produced.
type dumped struct {
	SchemaVersion int                       `json:"schema_version"`
	Version       string                    `json:"gitlab_version"`
	Revision      string                    `json:"gitlab_revision"`
	Entities      map[string]apilive.Entity `json:"entities"`
	Routes        []apilive.Route           `json:"routes"`
	Features      map[string]string         `json:"features"`
}

// runner boots a GitLab and runs the introspection inside it. It is a variable
// so a test exercises everything around the boot without one: the boot is the
// only part that needs Docker, and it is not the part that gets a rule wrong.
var runner = dockerRun

// interrupted is a context that ends on the first interrupt, so a boot a
// person gave up on tears its container down instead of leaving three
// gigabytes running. Seamed for the tests, which never wait on a signal.
var interrupted = func() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func main() {
	var (
		check = flag.Bool("check", false, "verify the committed record without Docker and without network, and exit non-zero when it cannot be rested on")
		dump  = flag.String("dump", "", "build the record from an introspection dump already on disk instead of booting")
		image = flag.String("image", "gitlab/gitlab-ee:latest", "image to boot; Enterprise, because its entity set is the superset")
		keep  = flag.Bool("keep", false, "leave the container running afterwards")
		dir   = flag.String("dir", "", "directory holding the record (default: the repository's docs/development)")
	)
	flag.Parse()

	recordDir := *dir
	if recordDir == "" {
		root := cmdutil.Must(cmdutil.RepositoryRoot("."))
		recordDir = filepath.Join(root, apilive.DefaultDir)
	}

	// Fatalf and not MustDo: both of these fail on something the person
	// running them acts on — regenerate the record, or boot a GitLab that
	// answers — and a stack trace over that message would bury it.
	if *check {
		if err := runCheck(recordDir, time.Now()); err != nil {
			cmdutil.Fatalf("%v", err)
		}
		return
	}
	if err := runGenerate(recordDir, *dump, *image, *keep); err != nil {
		cmdutil.Fatalf("%v", err)
	}
}

// runCheck gates the committed record. It reads one file and asks nothing of
// the network, because this is what CI and a contributor run.
func runCheck(dir string, now time.Time) error {
	doc, err := apilive.Read(dir)
	if err != nil {
		return err
	}

	problems := floorProblems(doc)
	problems = append(problems, provenance.Problems(provenance.Subject{
		Noun: "live API record",
		Consequence: "it can no longer say what a current GitLab sends, and the fields an audit " +
			"reports missing may be ones a newer release added or removed; regenerate it with make gen-api-live",
	}, doc.Source.RetrievedAt, now)...)

	if len(problems) > 0 {
		return fmt.Errorf("the live API record cannot be rested on:\n  %s", strings.Join(problems, "\n  "))
	}
	fmt.Printf("gen_api_live: %s\n", doc.Source)
	return nil
}

// floorProblems reports the ways the record is too small to be a GitLab.
func floorProblems(doc apilive.Document) []string {
	var problems []string
	for _, floor := range []struct {
		what  string
		got   int
		least int
	}{
		{"entities", len(doc.Entities), minEntities},
		{"exposed fields", doc.FieldCount(), minFields},
		{"routes", len(doc.Routes), minRoutes},
		{"licensed features", len(doc.Features), minFeatures},
	} {
		if floor.got < floor.least {
			problems = append(problems, fmt.Sprintf(
				"it holds %d %s and a GitLab has at least %d: the introspection did not finish, or it ran against something that is not a GitLab",
				floor.got, floor.what, floor.least,
			))
		}
	}
	// An entity that refused to describe itself is recorded rather than
	// dropped, so it is visible here rather than reading as one GitLab does
	// not have.
	var refused []string
	for _, name := range doc.Names() {
		if doc.Entities[name].Error != "" {
			refused = append(refused, name)
		}
	}
	if len(refused) > 0 {
		problems = append(problems, fmt.Sprintf(
			"%d entities refused to describe themselves (%s): the record understates what GitLab sends",
			len(refused), strings.Join(refused, ", "),
		))
	}
	return problems
}

// runGenerate produces the record, from a dump on disk or from a boot.
func runGenerate(dir, dumpPath, image string, keep bool) error {
	raw, source, err := introspection(dumpPath, image, keep)
	if err != nil {
		return err
	}

	var payload dumped
	if decodeErr := json.Unmarshal(raw, &payload); decodeErr != nil {
		return fmt.Errorf("decoding the introspection: %w", decodeErr)
	}
	if payload.SchemaVersion != apilive.SchemaVersion {
		return fmt.Errorf(
			"the introspection is schema version %d and this build writes version %d: the script and the command moved apart",
			payload.SchemaVersion, apilive.SchemaVersion,
		)
	}

	digest := sha256.Sum256(raw)
	doc := apilive.Document{
		SchemaVersion: apilive.SchemaVersion,
		Note: "What a booted GitLab says its own REST API is: every Grape entity with the fields it exposes " +
			"and the condition each is gated by, every mounted route with its declared entity and params, and " +
			"the licensed feature table. Generated by cmd/gen_api_live, which boots a released image and asks " +
			"the loaded application; do not edit by hand.",
		Source: apilive.Source{
			Image:       source.image,
			Digest:      source.digest,
			Version:     payload.Version,
			Revision:    payload.Revision,
			RetrievedAt: time.Now().UTC().Format(time.DateOnly),
			SHA256:      hex.EncodeToString(digest[:]),
			Entities:    len(payload.Entities),
			Routes:      len(payload.Routes),
			Features:    len(payload.Features),
		},
		Entities: payload.Entities,
		Routes:   payload.Routes,
		Features: payload.Features,
	}
	doc.Source.Fields = doc.FieldCount()

	if problems := floorProblems(doc); len(problems) > 0 {
		return fmt.Errorf("refusing to write a record that is not a GitLab:\n  %s", strings.Join(problems, "\n  "))
	}

	if writeErr := apilive.Write(dir, doc); writeErr != nil {
		return writeErr
	}
	fmt.Printf("gen_api_live: %s\n", doc.Source)
	return nil
}

// origin is where an introspection came from, for the record's provenance.
type origin struct {
	image  string
	digest string
}

// introspection returns the script's output, from a dump on disk when one is
// named and from a boot otherwise.
//
// A dump is not a convenience: it separates the part that needs Docker from
// the part that has rules in it, so the second can be tested and re-run
// without the first.
func introspection(dumpPath, image string, keep bool) ([]byte, origin, error) {
	if dumpPath != "" {
		raw, err := os.ReadFile(dumpPath)
		if err != nil {
			return nil, origin{}, fmt.Errorf("reading the introspection dump: %w", err)
		}
		return raw, origin{image: image}, nil
	}
	return runner(image, keep)
}

// containerName is fixed rather than random so a run that died leaves
// something a person can find and remove.
const containerName = "gitlab-mcp-api-live"

// dockerRun boots the image, waits for Rails, runs the script inside and
// returns its output.
//
// No license and no fixtures: the Enterprise classes load whatever the license
// says, because a license gates feature_available? when a request is served
// and not when a class is defined. That is what makes this cheap enough to run
// on every GitLab release.
func dockerRun(image string, keep bool) ([]byte, origin, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, origin{}, fmt.Errorf("gen_api_live needs docker to boot %s: %w", image, err)
	}

	ctx, stop := interrupted()
	defer stop()

	_ = exec.CommandContext(ctx, "docker", "rm", "-f", containerName).Run()
	boot := exec.CommandContext(ctx, "docker", "run", "-d", "--name", containerName,
		"--shm-size", "256m",
		// Everything a request needs is loaded by Rails; the rest is a boot
		// this waits on for no reason.
		"-e", "GITLAB_OMNIBUS_CONFIG=puma['worker_processes']=0; sidekiq['max_concurrency']=2; "+
			"prometheus_monitoring['enable']=false; gitlab_kas['enable']=false; registry['enable']=false;",
		image)
	if out, err := boot.CombinedOutput(); err != nil {
		return nil, origin{}, fmt.Errorf("booting %s: %w: %s", image, err, strings.TrimSpace(string(out)))
	}
	if !keep {
		// context.WithoutCancel: the teardown has to run even when the reason
		// we are unwinding is that the context ended, which is exactly the
		// interrupt case it exists for.
		defer func() {
			_ = exec.CommandContext(context.WithoutCancel(ctx), "docker", "rm", "-f", containerName).Run()
		}()
	}

	if err := waitForRails(ctx); err != nil {
		return nil, origin{}, err
	}

	script, err := os.CreateTemp("", "introspect-*.rb")
	if err != nil {
		return nil, origin{}, fmt.Errorf("staging the introspection script: %w", err)
	}
	defer func() { _ = os.Remove(script.Name()) }()
	if _, writeErr := script.WriteString(introspectScript); writeErr != nil {
		return nil, origin{}, fmt.Errorf("staging the introspection script: %w", writeErr)
	}
	if closeErr := script.Close(); closeErr != nil {
		return nil, origin{}, fmt.Errorf("staging the introspection script: %w", closeErr)
	}
	// #nosec G204 -- every argument is this command's own: a fixed container
	// name and a temp file it just created. The one value a caller supplies is
	// the image, and it is passed to docker run above rather than here.
	if out, copyErr := exec.CommandContext(ctx, "docker", "cp", script.Name(), containerName+":/tmp/introspect.rb").CombinedOutput(); copyErr != nil {
		return nil, origin{}, fmt.Errorf("copying the introspection script in: %w: %s", copyErr, strings.TrimSpace(string(out)))
	}

	// Only stdout is taken: Rails writes deprecation warnings to stderr and
	// they are not part of the answer.
	run := exec.CommandContext(ctx, "docker", "exec", containerName, "gitlab-rails", "runner", "/tmp/introspect.rb")
	out, err := run.Output()
	if err != nil {
		return nil, origin{}, fmt.Errorf("running the introspection: %w", err)
	}
	return out, origin{image: image, digest: imageDigest(ctx, image)}, nil
}

// bootTimeout bounds the wait for Rails. A first boot pulls three gigabytes
// and reconfigures; a second is under a minute.
const bootTimeout = 20 * time.Minute

// waitForRails polls until gitlab-rails runner answers, which is a stricter
// readiness than the container's own health check: the health check passes
// once the web server answers, and this needs the application loaded.
func waitForRails(ctx context.Context) error {
	deadline := time.Now().Add(bootTimeout)
	for time.Now().Before(deadline) {
		if err := exec.CommandContext(ctx, "docker", "exec", containerName, "gitlab-rails", "runner", "puts 1").Run(); err == nil {
			return nil
		}
		if !running(ctx) {
			out, _ := exec.CommandContext(ctx, "docker", "logs", "--tail", "20", containerName).CombinedOutput()
			return fmt.Errorf("the container stopped before the application was ready:\n%s", strings.TrimSpace(string(out)))
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for the application: %w", ctx.Err())
		case <-time.After(10 * time.Second):
		}
	}
	return fmt.Errorf("the application was not ready within %s: the container is up but gitlab-rails runner does not answer", bootTimeout)
}

// running reports whether the container is still up, so a boot that died is
// reported with its logs rather than waited out.
func running(ctx context.Context) bool {
	out, err := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", containerName).Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// imageDigest is the image's repository digest, which is what makes a
// regeneration reproducible: a tag moves and a digest does not. An image
// pulled without one reports none rather than failing the run.
func imageDigest(ctx context.Context, image string) string {
	out, err := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{index .RepoDigests 0}}", image).Output()
	if err != nil {
		return ""
	}
	digest := strings.TrimSpace(string(out))
	if _, after, found := strings.Cut(digest, "@"); found {
		return after
	}
	return ""
}
