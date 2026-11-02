package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"update-api/internal/api"
	"update-api/internal/repository"
)

const testKey = "test-admin-key"

// newServer wires the real handlers the way cmd/update-api does.
func newServer(t *testing.T) (*httptest.Server, *repository.SQLiteRepository) {
	t.Helper()
	repo, err := repository.NewSQLiteRepository(filepath.Join(t.TempDir(), "updates.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	if err := repo.Migrate(); err != nil {
		t.Fatal(err)
	}
	h := api.NewHandler(repo)
	adminAuth := api.AdminAuthMiddleware([]string{testKey})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/update-check", h.HandleUpdateCheck)
	mux.Handle("POST /v1/plugins", adminAuth(http.HandlerFunc(h.HandlePostPlugin)))
	mux.Handle("POST /v1/versions", adminAuth(http.HandlerFunc(h.HandlePostVersion)))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, repo
}

// writePlugin creates a minimal JCORE-style plugin directory.
func writePlugin(t *testing.T, slug string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), slug)
	files := map[string]string{
		slug + ".php":  "<?php\n/**\n * Plugin Name:       Example Plugin\n * Plugin URI:        https://github.com/JCO-Digital/" + slug + "\n * Version:           0.1.0\n */\n",
		"package.json": `{"name":"` + slug + `","repository":{"type":"git","url":"git+https://github.com/JCO-Digital/` + slug + `.git"}}`,
		".github/workflows/release.yml": "    uses: JCO-Digital/jcore-update/.github/workflows/reusable-plugin-publish.yml@main\n" +
			"    with:\n      slug: \"" + slug + "\"\n      version: ${{ needs.release.outputs.version }}\n" +
			"      dist_repo: \"git@git.example.test:/srv/git/" + slug + ".git\"\n",
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, strings.NewReader(""), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestReadPlugin(t *testing.T) {
	dir := writePlugin(t, "jcore-example")
	info, err := readPlugin(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Slug != "jcore-example" || info.Name != "Example Plugin" {
		t.Errorf("got slug %q name %q", info.Slug, info.Name)
	}
	if info.RepoURL != "https://github.com/JCO-Digital/jcore-example" {
		t.Errorf("RepoURL = %q", info.RepoURL)
	}
	if info.WorkflowSlug != "jcore-example" || info.DistRepo != "git@git.example.test:/srv/git/jcore-example.git" || info.DistDeploy {
		t.Errorf("workflow slug %q dist %q deploy %v", info.WorkflowSlug, info.DistRepo, info.DistDeploy)
	}
	if len(info.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", info.Warnings)
	}
}

func TestNextStepsFollowDistDeploy(t *testing.T) {
	dir := writePlugin(t, "jcore-example")
	api, _ := parseAPI(defaultAPI)
	steps := func() string {
		info, err := readPlugin(dir)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		printNextSteps(&out, info, api, true)
		return out.String()
	}
	if s := steps(); !strings.Contains(s, "Dist deploy is off") {
		t.Errorf("without dist_deploy:\n%s", s)
	}
	wf := filepath.Join(dir, ".github", "workflows", "release.yml")
	b, _ := os.ReadFile(wf)
	os.WriteFile(wf, append(b, []byte("      dist_deploy: true\n")...), 0o644)
	if s := steps(); !strings.Contains(s, "must exist (a bare repo)") {
		t.Errorf("with dist_deploy: true:\n%s", s)
	}
}

func TestVersionFlag(t *testing.T) {
	code, out, _ := runCLI(t, "--version")
	if code != 0 || out != "jcore-register-plugin dev\n" {
		t.Errorf("exit %d: %q", code, out)
	}
}

func TestHeaderFieldStopsAtCommentEnd(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.php")
	os.WriteFile(p, []byte("<?php /* Plugin Name: One Liner */\n"), 0o644)
	if got := headerField(p, "Plugin Name"); got != "One Liner" {
		t.Errorf("got %q", got)
	}
}

func TestRegisterFlow(t *testing.T) {
	srv, repo := newServer(t)
	dir := writePlugin(t, "jcore-example")
	apiURL := srv.URL + "/v1"
	t.Setenv("UPDATE_API_KEY", testKey)

	// Dry run sends nothing and never prints the key.
	code, out, errOut := runCLI(t, "--api", apiURL, "--dry-run", dir)
	if code != 0 {
		t.Fatalf("dry run exit %d: %s", code, errOut)
	}
	if strings.Contains(out+errOut, testKey) {
		t.Fatal("dry run printed the API key")
	}
	if !strings.Contains(out, `"slug": "jcore-example"`) || !strings.Contains(out, "<redacted, from $UPDATE_API_KEY>") {
		t.Errorf("dry run output missing request:\n%s", out)
	}
	if p, _ := repo.GetPlugin("jcore-example"); p != nil {
		t.Fatal("dry run registered the plugin")
	}

	// Real run registers it.
	code, out, errOut = runCLI(t, "--api", apiURL, dir)
	if code != 0 {
		t.Fatalf("register exit %d: %s", code, errOut)
	}
	p, _ := repo.GetPlugin("jcore-example")
	if p == nil || p.Name != "Example Plugin" || p.IsPaid {
		t.Fatalf("plugin after register: %+v", p)
	}
	if !strings.Contains(out, "Confirmed: jcore-example is registered, no versions published yet") {
		t.Errorf("missing confirmation:\n%s", out)
	}
	if strings.Contains(out+errOut, testKey) {
		t.Fatal("output contains the API key")
	}

	// A version can now be published, as the release workflow does.
	repo.SaveVersion(repository.Version{PluginSlug: "jcore-example", Version: "0.2.0", DownloadURL: "https://example.test/x.zip"})

	// Second run is a no-op that reports the existing registration.
	code, out, _ = runCLI(t, "--api", apiURL, "--name", "Renamed", dir)
	if code != 0 || !strings.Contains(out, "Already registered: jcore-example is registered, latest version 0.2.0") {
		t.Fatalf("second run exit %d:\n%s", code, out)
	}
	if p, _ := repo.GetPlugin("jcore-example"); p.Name != "Example Plugin" {
		t.Errorf("second run changed the plugin: %+v", p)
	}
}

func TestPaidPluginIsNotOverwritten(t *testing.T) {
	srv, repo := newServer(t)
	repo.SavePlugin(repository.Plugin{Slug: "jcore-paid", Name: "Paid", Secret: "keep-me", IsPaid: true})
	t.Setenv("UPDATE_API_KEY", testKey)

	code, out, _ := runCLI(t, "--api", srv.URL+"/v1", "--slug", "jcore-paid", "--name", "Paid")
	if code != 0 || !strings.Contains(out, "paid plugin") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if p, _ := repo.GetPlugin("jcore-paid"); p.Secret != "keep-me" || !p.IsPaid {
		t.Errorf("paid plugin was modified: %+v", p)
	}
}

func TestPaidRegistrationGeneratesHiddenSecret(t *testing.T) {
	srv, repo := newServer(t)
	t.Setenv("UPDATE_API_KEY", testKey)

	code, out, errOut := runCLI(t, "--api", srv.URL+"/v1", "--slug", "jcore-new-paid", "--name", "New Paid", "--paid")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	p, _ := repo.GetPlugin("jcore-new-paid")
	if p == nil || !p.IsPaid || len(p.Secret) != 64 {
		t.Fatalf("plugin: %+v", p)
	}
	if strings.Contains(out+errOut, p.Secret) {
		t.Error("output contains the generated secret")
	}
}

func TestErrors(t *testing.T) {
	srv, repo := newServer(t)
	apiURL := srv.URL + "/v1"

	t.Run("no key", func(t *testing.T) {
		t.Setenv("UPDATE_API_KEY", "")
		t.Setenv("UPDATE_API_KEY_FILE", "")
		code, _, errOut := runCLI(t, "--api", apiURL, "--slug", "jcore-a", "--name", "A")
		if code != 1 || !strings.Contains(errOut, "no API key") {
			t.Errorf("exit %d: %s", code, errOut)
		}
	})

	t.Run("wrong key", func(t *testing.T) {
		t.Setenv("UPDATE_API_KEY", "wrong-key")
		code, out, errOut := runCLI(t, "--api", apiURL, "--slug", "jcore-a", "--name", "A")
		if code != 1 || !strings.Contains(errOut, "rejected the key") || strings.Contains(out+errOut, "wrong-key") {
			t.Errorf("exit %d: %s", code, errOut)
		}
		if p, _ := repo.GetPlugin("jcore-a"); p != nil {
			t.Error("registered with a wrong key")
		}
	})

	t.Run("key file", func(t *testing.T) {
		t.Setenv("UPDATE_API_KEY", "")
		f := filepath.Join(t.TempDir(), "key")
		os.WriteFile(f, []byte(testKey+"\n"), 0o600)
		code, _, errOut := runCLI(t, "--api", apiURL, "--key-file", f, "--slug", "jcore-b", "--name", "B")
		if code != 0 {
			t.Errorf("exit %d: %s", code, errOut)
		}
	})

	t.Run("wrong base URL is not mistaken for an unknown slug", func(t *testing.T) {
		t.Setenv("UPDATE_API_KEY", testKey)
		code, _, errOut := runCLI(t, "--api", srv.URL+"/v2", "--slug", "jcore-c", "--name", "C")
		if code != 1 || !strings.Contains(errOut, "is the API URL right?") {
			t.Errorf("exit %d: %s", code, errOut)
		}
	})

	t.Run("plain http to a remote host", func(t *testing.T) {
		code, _, errOut := runCLI(t, "--api", "http://update.example.test/v1", "--slug", "jcore-d", "--name", "D", "--dry-run")
		if code != 1 || !strings.Contains(errOut, "plain http") {
			t.Errorf("exit %d: %s", code, errOut)
		}
	})

	t.Run("bad slug", func(t *testing.T) {
		code, _, errOut := runCLI(t, "--api", apiURL, "--slug", "Bad_Slug", "--name", "X")
		if code != 1 || !strings.Contains(errOut, "invalid slug") {
			t.Errorf("exit %d: %s", code, errOut)
		}
	})
}

func TestConfirm(t *testing.T) {
	var out bytes.Buffer
	for input, want := range map[string]bool{"y\n": true, "yes\n": true, "\n": false, "n\n": false, "": false} {
		if got, _ := confirm(strings.NewReader(input), &out, "Register?"); got != want {
			t.Errorf("confirm(%q) = %v, want %v", input, got, want)
		}
	}
	// A pipe on stdin is not a terminal: refuse rather than guess. (/dev/null
	// passes the char-device check, but reads as an empty answer, so no.)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	w.WriteString("y\n")
	w.Close()
	if ok, err := confirm(r, &out, "Register?"); ok || err == nil {
		t.Errorf("non-terminal stdin: ok=%v err=%v", ok, err)
	}
}
