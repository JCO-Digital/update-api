// Package importer rebuilds plugins and versions from GitHub releases.
//
// It mirrors what jcore-update's reusable publish workflow POSTs to
// /v1/versions: for every release of a repo that has <slug>.zip attached, the
// zip's URL, the release notes as changelog, and the requirements read from
// the plugin header at that tag.
package importer

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"update-api/internal/repository"

	"github.com/Masterminds/semver/v3"
)

type Store interface {
	EnsurePlugin(p repository.Plugin) (bool, error)
	SaveVersion(v repository.Version) error
}

type GitHub struct {
	Org   string
	Topic string
	// Token is optional for public repos but raises the API rate limit.
	Token string

	HTTP    *http.Client
	APIBase string
	RawBase string
	Logf    func(format string, args ...any)
}

type Summary struct {
	Repos          int
	PluginsCreated int
	Versions       int
	Skipped        int
}

type ghRepo struct {
	Name string `json:"name"`
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Body        string    `json:"body"`
	Draft       bool      `json:"draft"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func (g *GitHub) defaults() {
	if g.HTTP == nil {
		g.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	if g.APIBase == "" {
		g.APIBase = "https://api.github.com"
	}
	if g.RawBase == "" {
		g.RawBase = "https://raw.githubusercontent.com"
	}
	if g.Logf == nil {
		g.Logf = func(string, ...any) {}
	}
}

// Run imports every repo in Org with Topic. With dryRun it only logs what it
// would write. Existing plugins keep their name, secret and paid flag; new
// ones are created free and without a secret.
func (g *GitHub) Run(ctx context.Context, store Store, dryRun bool) (Summary, error) {
	g.defaults()
	var sum Summary

	repos, err := g.repos(ctx)
	if err != nil {
		return sum, err
	}
	sum.Repos = len(repos)

	for _, slug := range repos {
		releases, err := g.releases(ctx, slug)
		if err != nil {
			return sum, fmt.Errorf("%s: %w", slug, err)
		}

		// Oldest first, so version ids keep release order.
		sort.Slice(releases, func(i, j int) bool {
			return releases[i].PublishedAt.Before(releases[j].PublishedAt)
		})

		var versions []repository.Version
		var name string
		for _, rel := range releases {
			if rel.Draft {
				continue
			}
			var zipURL string
			for _, a := range rel.Assets {
				if a.Name == slug+".zip" {
					zipURL = a.BrowserDownloadURL
				}
			}
			if zipURL == "" {
				continue
			}

			ver := strings.TrimPrefix(rel.TagName, "v")
			if _, err := semver.NewVersion(ver); err != nil {
				g.Logf("  skip %s %s: not SemVer", slug, rel.TagName)
				sum.Skipped++
				continue
			}

			header, err := g.pluginHeader(ctx, slug, rel.TagName)
			if err != nil {
				// Not every repo keeps the plugin at the root; the zip
				// always has it, it's just a bigger download.
				header, err = g.zipPluginHeader(ctx, slug, zipURL)
			}
			if err != nil {
				g.Logf("  warn %s %s: no plugin header (%v), requirements left empty", slug, rel.TagName, err)
			}
			if header["Plugin Name"] != "" {
				name = header["Plugin Name"]
			}

			versions = append(versions, repository.Version{
				PluginSlug:  slug,
				Version:     ver,
				DownloadURL: zipURL,
				RequiresWP:  header["Requires at least"],
				TestedWP:    header["Tested up to"],
				RequiresPHP: header["Requires PHP"],
				Changelog:   rel.Body,
				CreatedAt:   rel.PublishedAt.UTC().Format("2006-01-02 15:04:05"),
			})
		}

		if len(versions) == 0 {
			g.Logf("%s: no releases with %s.zip, skipped", slug, slug)
			continue
		}
		if name == "" {
			name = slug
		}

		g.Logf("%s (%s): %d versions, %s to %s", slug, name, len(versions), versions[0].Version, versions[len(versions)-1].Version)
		if dryRun {
			sum.Versions += len(versions)
			continue
		}

		created, err := store.EnsurePlugin(repository.Plugin{Slug: slug, Name: name})
		if err != nil {
			return sum, fmt.Errorf("%s: %w", slug, err)
		}
		if created {
			sum.PluginsCreated++
		}
		for _, v := range versions {
			if err := store.SaveVersion(v); err != nil {
				return sum, fmt.Errorf("%s %s: %w", slug, v.Version, err)
			}
			sum.Versions++
		}
	}
	return sum, nil
}

func (g *GitHub) repos(ctx context.Context) ([]string, error) {
	q := url.Values{}
	q.Set("q", fmt.Sprintf("org:%s topic:%s", g.Org, g.Topic))
	q.Set("per_page", "100")

	var names []string
	for page := 1; ; page++ {
		q.Set("page", fmt.Sprint(page))
		var res struct {
			Items []ghRepo `json:"items"`
		}
		if err := g.getJSON(ctx, g.APIBase+"/search/repositories?"+q.Encode(), &res); err != nil {
			return nil, err
		}
		for _, r := range res.Items {
			names = append(names, r.Name)
		}
		if len(res.Items) < 100 {
			break
		}
	}
	sort.Strings(names)
	return names, nil
}

func (g *GitHub) releases(ctx context.Context, repo string) ([]ghRelease, error) {
	var all []ghRelease
	for page := 1; ; page++ {
		var rels []ghRelease
		u := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=100&page=%d", g.APIBase, g.Org, repo, page)
		if err := g.getJSON(ctx, u, &rels); err != nil {
			return nil, err
		}
		all = append(all, rels...)
		if len(rels) < 100 {
			return all, nil
		}
	}
}

// pluginHeader reads the header of <slug>.php in the repo root at tag. Raw
// file downloads don't count against the API rate limit.
func (g *GitHub) pluginHeader(ctx context.Context, slug, tag string) (map[string]string, error) {
	u := fmt.Sprintf("%s/%s/%s/%s/%s.php", g.RawBase, g.Org, slug, url.PathEscape(tag), slug)
	body, err := g.get(ctx, u)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return parseHeader(body)
}

// zipPluginHeader reads the header of <slug>/<slug>.php from the release zip.
func (g *GitHub) zipPluginHeader(ctx context.Context, slug, zipURL string) (map[string]string, error) {
	body, err := g.get(ctx, zipURL)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	data, err := io.ReadAll(io.LimitReader(body, maxZipSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxZipSize {
		return nil, fmt.Errorf("%s is larger than %d bytes", zipURL, maxZipSize)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, name := range []string{slug + "/" + slug + ".php", slug + ".php"} {
		f, err := zr.Open(name)
		if err != nil {
			continue
		}
		defer f.Close()
		return parseHeader(f)
	}
	return nil, fmt.Errorf("%s.php not found in %s", slug, zipURL)
}

const maxZipSize = 200 << 20

// parseHeader reads the "Key: value" lines of a plugin file's header comment.
func parseHeader(r io.Reader) (map[string]string, error) {
	header := map[string]string{}
	sc := bufio.NewScanner(io.LimitReader(r, 8192))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "*/") {
			break
		}
		line = strings.TrimSpace(strings.TrimLeft(line, "/*#"))
		key, val, ok := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		if ok && header[key] == "" {
			header[key] = strings.TrimSpace(val)
		}
	}
	return header, sc.Err()
}

func (g *GitHub) getJSON(ctx context.Context, u string, out any) error {
	body, err := g.get(ctx, u)
	if err != nil {
		return err
	}
	defer body.Close()
	return json.NewDecoder(body).Decode(out)
}

func (g *GitHub) get(ctx context.Context, u string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}

	resp, err := g.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s: %s", u, resp.Status, strings.TrimSpace(string(msg)))
	}
	return resp.Body, nil
}
