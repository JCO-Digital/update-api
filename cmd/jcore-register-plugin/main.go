// Command jcore-register-plugin registers a plugin slug with a running update-api
// over HTTP, so the plugin's first release can POST /v1/versions and sites can
// find it through /v1/update-check.
//
// Unlike update-cli, which writes to the SQLite database directly and so only
// works on the server, this is a client: run it from a developer machine
// against https://update.jcore.fi/v1 (the default) or a local server.
//
// It reads the slug, name and repository from a plugin directory, checks
// whether the slug is already known, and only then sends POST /v1/plugins.
// An already-registered slug is reported and left alone, because the API's
// upsert replaces every field and would clear a paid plugin's secret.
package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const defaultAPI = "https://update.jcore.fi/v1"

// version is set at build time: -ldflags "-X main.version=1.2.0".
var version = "dev"

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

type options struct {
	dir        string
	slug       string
	name       string
	api        string
	keyFile    string
	secretFile string
	paid       bool
	dryRun     bool
	yes        bool
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("jcore-register-plugin", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o options
	fs.StringVar(&o.slug, "slug", "", "Plugin slug (default: the plugin directory's name)")
	fs.StringVar(&o.name, "name", "", "Display name (default: the main file's Plugin Name header)")
	fs.StringVar(&o.api, "api", envOr("UPDATE_API_URL", defaultAPI), "API base URL, including /v1 ($UPDATE_API_URL)")
	fs.StringVar(&o.keyFile, "key-file", os.Getenv("UPDATE_API_KEY_FILE"), "File holding the admin API key ($UPDATE_API_KEY_FILE); $UPDATE_API_KEY wins if set")
	fs.BoolVar(&o.paid, "paid", false, "Register as a paid plugin (update checks need a license key)")
	fs.StringVar(&o.secretFile, "secret-file", "", "With --paid: file holding the license signing secret ($UPDATE_PLUGIN_SECRET); a random one is generated if neither is set")
	fs.BoolVar(&o.dryRun, "dry-run", false, "Check registration and print the request instead of sending it")
	fs.BoolVar(&o.yes, "yes", false, "Don't ask before writing to a non-local API")
	showVersion := fs.Bool("version", false, "Print the version and exit")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: jcore-register-plugin [flags] [plugin-dir]\n\n")
		fmt.Fprintf(stderr, "Registers a plugin slug with update-api (POST /v1/plugins) unless it is already registered.\n")
		fmt.Fprintf(stderr, "The admin API key is read from $UPDATE_API_KEY or --key-file, never from a flag value.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "jcore-register-plugin %s\n", version)
		return 0
	}
	switch fs.NArg() {
	case 0:
		if o.slug == "" || o.name == "" {
			o.dir = "."
		}
	case 1:
		o.dir = fs.Arg(0)
	default:
		fs.Usage()
		return 2
	}

	if err := register(o, stdin, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

func register(o options, stdin io.Reader, stdout, stderr io.Writer) error {
	info := pluginInfo{}
	if o.dir != "" {
		var err error
		info, err = readPlugin(o.dir)
		if err != nil {
			return err
		}
		for _, w := range info.Warnings {
			fmt.Fprintf(stderr, "Warning: %s\n", w)
		}
	}
	if o.slug != "" {
		if info.Slug != "" && o.slug != info.Slug {
			fmt.Fprintf(stderr, "Warning: --slug %s differs from the plugin directory %s; jcore-update expects them to match\n", o.slug, info.Slug)
		}
		info.Slug = o.slug
	}
	if o.name != "" {
		info.Name = o.name
	}
	if !slugPattern.MatchString(info.Slug) {
		return fmt.Errorf("invalid slug %q: use lowercase letters, digits and single hyphens", info.Slug)
	}
	if info.Name == "" {
		return errors.New("no plugin name: pass --name or a directory whose main file has a Plugin Name header")
	}

	api, err := parseAPI(o.api)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 15 * time.Second}

	fmt.Fprintf(stdout, "Plugin:  %s (%s)\n", info.Slug, info.Name)
	if info.MainFile != "" {
		fmt.Fprintf(stdout, "Source:  %s\n", info.MainFile)
	}
	fmt.Fprintf(stdout, "API:     %s\n\n", api)

	state, err := checkRegistered(client, api, info.Slug)
	if err != nil {
		if !o.dryRun {
			return fmt.Errorf("could not tell whether %s is registered, so nothing was sent: %w", info.Slug, err)
		}
		fmt.Fprintf(stderr, "Warning: registration check failed: %v\n", err)
	} else if state.Registered {
		fmt.Fprintf(stdout, "Already registered: %s. Nothing to do.\n", state.describe(info.Slug))
		printNextSteps(stdout, info, api, true)
		return nil
	} else {
		fmt.Fprintf(stdout, "Not registered yet: %s returns \"plugin not found\".\n", info.Slug)
	}

	body := registerRequest{Slug: info.Slug, Name: info.Name, IsPaid: o.paid}
	secretNote := ""
	if o.paid {
		body.Secret, secretNote, err = licenseSecret(o.secretFile)
		if err != nil {
			return err
		}
	}

	key, keySource, keyErr := apiKey(o.keyFile)
	endpoint := api.JoinPath("plugins").String()

	if o.dryRun {
		fmt.Fprintf(stdout, "\nDry run, this request would be sent:\n\n")
		fmt.Fprintf(stdout, "POST %s\nContent-Type: application/json\n", endpoint)
		if keyErr != nil {
			fmt.Fprintf(stdout, "X-API-Key: <missing: %v>\n", keyErr)
		} else {
			fmt.Fprintf(stdout, "X-API-Key: <redacted, from %s>\n", keySource)
		}
		shown := body
		if shown.Secret != "" {
			shown.Secret = "<redacted>"
		}
		out, _ := json.MarshalIndent(shown, "", "  ")
		fmt.Fprintf(stdout, "\n%s\n", out)
		if secretNote != "" {
			fmt.Fprintf(stdout, "(%s)\n", secretNote)
		}
		printNextSteps(stdout, info, api, false)
		return nil
	}

	if keyErr != nil {
		return keyErr
	}
	if !isLocal(api) && !o.yes {
		ok, err := confirm(stdin, stdout, fmt.Sprintf("Register %s on %s?", info.Slug, api.Host))
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("aborted, nothing was sent")
		}
	}

	if err := postPlugin(client, endpoint, key, body); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Registered %s on %s.\n", info.Slug, api.Host)
	if secretNote != "" {
		fmt.Fprintf(stdout, "(%s)\n", secretNote)
	}

	// Read back through the public endpoint, the way sites will see it.
	if state, err := checkRegistered(client, api, info.Slug); err != nil {
		fmt.Fprintf(stderr, "Warning: could not confirm the registration: %v\n", err)
	} else if !state.Registered {
		return fmt.Errorf("the API accepted the request but %s still reports \"plugin not found\"", info.Slug)
	} else {
		fmt.Fprintf(stdout, "Confirmed: %s.\n", state.describe(info.Slug))
	}
	printNextSteps(stdout, info, api, true)
	return nil
}

// registerRequest is the body of POST /v1/plugins (repository.Plugin).
type registerRequest struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Secret string `json:"secret,omitempty"`
	IsPaid bool   `json:"is_paid"`
}

func postPlugin(client *http.Client, endpoint, key string, body registerRequest) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", key)

	resp, err := client.Do(req)
	if err != nil {
		// url.Error carries the URL and cause, never the headers.
		return fmt.Errorf("POST %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	msg := readSnippet(resp.Body)

	switch {
	case resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK:
		return nil
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("the API rejected the key (401: %s)", msg)
	default:
		return fmt.Errorf("POST %s returned %d: %s", endpoint, resp.StatusCode, msg)
	}
}

type registration struct {
	Registered bool
	Paid       bool
	Latest     string // newest published version, if any and readable
}

func (r registration) describe(slug string) string {
	switch {
	case r.Paid:
		return slug + " is a paid plugin (update checks need a license key)"
	case r.Latest != "":
		return slug + " is registered, latest version " + r.Latest
	default:
		return slug + " is registered, no versions published yet"
	}
}

// checkRegistered asks the public update-check endpoint about the slug. The
// API has no read endpoint for plugins, but update-check answers "plugin not
// found" (404) for an unknown slug, 403 for a paid one without a license, and
// 200 or 204 for a free one.
func checkRegistered(client *http.Client, api *url.URL, slug string) (registration, error) {
	u := api.JoinPath("update-check")
	u.RawQuery = url.Values{"slug": {slug}, "version": {"0.0.0"}}.Encode()

	resp, err := client.Get(u.String())
	if err != nil {
		return registration{}, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var v struct {
			NewVersion string `json:"new_version"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&v)
		return registration{Registered: true, Latest: v.NewVersion}, nil
	case http.StatusNoContent:
		return registration{Registered: true}, nil
	case http.StatusForbidden:
		return registration{Registered: true, Paid: true}, nil
	case http.StatusNotFound:
		msg := readSnippet(resp.Body)
		if strings.Contains(msg, "plugin not found") {
			return registration{}, nil
		}
		// Anything else is a wrong URL, not an unknown slug.
		return registration{}, fmt.Errorf("GET %s returned 404 %q; is the API URL right?", u.Redacted(), msg)
	default:
		return registration{}, fmt.Errorf("GET %s returned %d: %s", u.Redacted(), resp.StatusCode, readSnippet(resp.Body))
	}
}

// apiKey returns the admin key and where it came from. The key is never
// accepted as a flag value, so it can't end up in shell history or ps.
func apiKey(keyFile string) (key, source string, err error) {
	if k := strings.TrimSpace(os.Getenv("UPDATE_API_KEY")); k != "" {
		return k, "$UPDATE_API_KEY", nil
	}
	if keyFile == "" {
		return "", "", errors.New("no API key: set UPDATE_API_KEY or pass --key-file (or UPDATE_API_KEY_FILE)")
	}
	k, err := readSecretFile(keyFile)
	if err != nil {
		return "", "", fmt.Errorf("reading the API key: %w", err)
	}
	return k, keyFile, nil
}

func licenseSecret(secretFile string) (secret, note string, err error) {
	if s := strings.TrimSpace(os.Getenv("UPDATE_PLUGIN_SECRET")); s != "" {
		return s, "license secret from $UPDATE_PLUGIN_SECRET", nil
	}
	if secretFile != "" {
		s, err := readSecretFile(secretFile)
		if err != nil {
			return "", "", fmt.Errorf("reading the license secret: %w", err)
		}
		return s, "license secret from " + secretFile, nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	return hex.EncodeToString(b), "a random license secret was generated; it is kept only on the server, which signs license keys itself", nil
}

func readSecretFile(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if fi.Mode().Perm()&0o077 != 0 {
		fmt.Fprintf(os.Stderr, "Warning: %s is readable by other users (mode %04o); chmod 600 it\n", path, fi.Mode().Perm())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "", fmt.Errorf("%s is empty", path)
	}
	return s, nil
}

func parseAPI(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("invalid API URL %q: want something like %s", raw, defaultAPI)
	}
	if u.User != nil || u.RawQuery != "" {
		return nil, fmt.Errorf("invalid API URL %q: no credentials or query string", u.Redacted())
	}
	if u.Scheme == "http" && !isLocal(u) {
		return nil, fmt.Errorf("refusing to send the API key over plain http to %s; use https", u.Host)
	}
	return u, nil
}

func isLocal(u *url.URL) bool {
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func confirm(stdin io.Reader, stdout io.Writer, question string) (bool, error) {
	if f, ok := stdin.(*os.File); ok {
		if fi, err := f.Stat(); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
			return false, errors.New("not a terminal, so can't ask for confirmation: re-run with --yes to write to a non-local API")
		}
	}
	fmt.Fprintf(stdout, "%s [y/N] ", question)
	line, _ := bufio.NewReader(stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

// pluginInfo is what readPlugin finds in a plugin directory.
type pluginInfo struct {
	Dir      string
	Slug     string
	Name     string
	MainFile string
	RepoURL  string // https URL of the source repository
	Remote   string // the local checkout's origin, if any
	// From .github/workflows/release.yml, if it calls the publish workflow.
	WorkflowSlug string
	DistRepo     string
	DistDeploy   bool
	Warnings     []string
}

func readPlugin(dir string) (pluginInfo, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return pluginInfo{}, err
	}
	info := pluginInfo{Dir: abs, Slug: filepath.Base(abs)}

	main, name, err := findMainFile(abs, info.Slug)
	if err != nil {
		return pluginInfo{}, err
	}
	info.MainFile, info.Name = main, name
	if base := strings.TrimSuffix(filepath.Base(main), ".php"); base != info.Slug {
		info.Warnings = append(info.Warnings, fmt.Sprintf("main file %s doesn't match the directory name %s", filepath.Base(main), info.Slug))
	}
	pluginURI := headerField(main, "Plugin URI")

	if b, err := os.ReadFile(filepath.Join(abs, "package.json")); err == nil {
		var pkg struct {
			Name       string          `json:"name"`
			Repository json.RawMessage `json:"repository"`
		}
		if err := json.Unmarshal(b, &pkg); err != nil {
			info.Warnings = append(info.Warnings, "package.json: "+err.Error())
		} else {
			info.RepoURL = repoURL(pkg.Repository)
			if pkg.Name != "" && pkg.Name != info.Slug {
				info.Warnings = append(info.Warnings, fmt.Sprintf("package.json name %q differs from the slug %s", pkg.Name, info.Slug))
			}
		}
	}
	if info.RepoURL == "" && strings.Contains(pluginURI, "github.com/") {
		info.RepoURL = strings.TrimSuffix(pluginURI, "/")
	}

	if out, err := exec.Command("git", "-C", abs, "remote", "get-url", "origin").Output(); err == nil {
		info.Remote = strings.TrimSpace(string(out))
	}

	if b, err := os.ReadFile(filepath.Join(abs, ".github", "workflows", "release.yml")); err == nil && bytes.Contains(b, []byte("reusable-plugin-publish.yml")) {
		info.WorkflowSlug = yamlScalar(b, "slug")
		info.DistRepo = yamlScalar(b, "dist_repo")
		info.DistDeploy = yamlScalar(b, "dist_deploy") == "true"
		if info.WorkflowSlug != "" && info.WorkflowSlug != info.Slug {
			info.Warnings = append(info.Warnings, fmt.Sprintf("release.yml publishes slug %q, not %s", info.WorkflowSlug, info.Slug))
		}
	} else if err == nil {
		info.Warnings = append(info.Warnings, "release.yml doesn't call jcore-update's reusable-plugin-publish.yml")
	}
	return info, nil
}

// findMainFile prefers <slug>.php, then any top-level PHP file with a Plugin
// Name header, as WordPress does.
func findMainFile(dir, slug string) (path, name string, err error) {
	preferred := filepath.Join(dir, slug+".php")
	if n := headerField(preferred, "Plugin Name"); n != "" {
		return preferred, n, nil
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.php"))
	sort.Strings(matches)
	for _, m := range matches {
		if n := headerField(m, "Plugin Name"); n != "" {
			return m, n, nil
		}
	}
	return "", "", fmt.Errorf("%s has no PHP file with a Plugin Name header", dir)
}

// headerField reads a plugin header field the way WordPress's get_file_data
// does: from the first 8 KiB, one field per line, comment markers ignored.
func headerField(path, field string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	head, _ := io.ReadAll(io.LimitReader(f, 8192))
	re := regexp.MustCompile(`(?mi)^(?:[ \t]*<\?php)?[ \t/*#@]*` + regexp.QuoteMeta(field) + `:(.*)$`)
	m := re.FindSubmatch(head)
	if m == nil {
		return ""
	}
	v := string(m[1])
	if i := strings.Index(v, "*/"); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}

var yamlKey = map[string]*regexp.Regexp{}

// yamlScalar returns the first `key: value` in a workflow file. Enough for the
// flat `with:` block of the release workflow; not a YAML parser.
func yamlScalar(b []byte, key string) string {
	re, ok := yamlKey[key]
	if !ok {
		re = regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(key) + `:\s*["']?([^"'\s#]+)`)
		yamlKey[key] = re
	}
	m := re.FindSubmatch(b)
	if m == nil || bytes.Contains(m[1], []byte("${{")) {
		return ""
	}
	return string(m[1])
}

// repoURL turns package.json's repository (a string or {url}) into an https URL.
func repoURL(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		var obj struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(raw, &obj) != nil {
			return ""
		}
		s = obj.URL
	}
	s = strings.TrimPrefix(s, "git+")
	s = strings.TrimSuffix(s, ".git")
	if strings.HasPrefix(s, "git@github.com:") {
		s = "https://github.com/" + strings.TrimPrefix(s, "git@github.com:")
	}
	if strings.HasPrefix(s, "github:") {
		s = "https://github.com/" + strings.TrimPrefix(s, "github:")
	}
	return s
}

// ghRepo returns "owner/repo" for a GitHub URL, or "".
func ghRepo(u string) string {
	rest, ok := strings.CutPrefix(u, "https://github.com/")
	if !ok {
		return ""
	}
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

// printNextSteps lists the setup this tool deliberately doesn't automate:
// each step touches GitHub settings, a git server or secrets.
func printNextSteps(w io.Writer, info pluginInfo, api *url.URL, registered bool) {
	slug := info.Slug
	repo := ghRepo(info.RepoURL)
	if repo == "" {
		repo = "JCO-Digital/" + slug
	}
	n := 0
	step := func(format string, args ...any) {
		n++
		fmt.Fprintf(w, "  %d. "+format+"\n", append([]any{n}, args...)...)
	}

	fmt.Fprintf(w, "\nStill manual (not automated by this tool):\n")
	if !registered {
		step("Run this again without --dry-run to register %s.", slug)
	}
	if info.Remote == "" {
		step("Create the GitHub repo %s and push main: gh repo create %s --private --source %s --push", repo, repo, displayDir(info.Dir))
	} else {
		step("GitHub repo: origin is %s. Make sure main is pushed.", info.Remote)
	}
	step("Add the GitHub topic jcore-plugin so `update-cli import` can rebuild it: gh repo edit %s --add-topic jcore-plugin", repo)
	step("Repo secrets UPDATE_API_KEY, DIST_REPO_SSH_KEY and SLACK_BOT_TOKEN. Check for org-level ones first:\n"+
		"       gh secret list --org %s; gh secret list -R %s", strings.Split(repo, "/")[0], repo)
	switch {
	case info.Dir != "" && info.WorkflowSlug == "":
		step("Add .github/workflows/release.yml calling jcore-update's reusable-plugin-publish.yml.")
	case info.DistDeploy:
		step("Dist repo %s must exist (a bare repo) and accept pushes from DIST_REPO_SSH_KEY.", info.DistRepo)
	case info.DistRepo != "":
		step("Dist deploy is off: release.yml sets dist_repo but not dist_deploy: true, so releases skip it.\n"+
			"       If sites install this through Composer, add dist_deploy: true and create %s.", info.DistRepo)
	}
	step("Branch rules on main must let the release workflow's token push foonver's version commit and tag.")
	check := api.JoinPath("update-check")
	check.RawQuery = url.Values{"slug": {slug}, "version": {"0.0.0"}}.Encode()
	step("After the first merge to main, check the publish job's \"Publish to Update API\" step shows HTTP 201\n"+
		"       (it doesn't fail the job on errors), then: curl -i '%s'", check)
}

func displayDir(dir string) string {
	if dir == "" {
		return "."
	}
	if home, err := os.UserHomeDir(); err == nil {
		if rel, ok := strings.CutPrefix(dir, home+string(os.PathSeparator)); ok {
			return "~/" + rel
		}
	}
	return dir
}

func readSnippet(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 300))
	return strings.TrimSpace(string(b))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
