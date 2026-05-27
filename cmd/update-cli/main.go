package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"
	"update-api/internal/auth"
	"update-api/internal/config"
	"update-api/internal/repository"
)

func main() {
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		cfg, err = config.LoadConfig("config.yaml.dist")
		if err != nil {
			log.Fatalf("Error loading config: %v", err)
		}
	}

	repo, err := repository.NewSQLiteRepository(cfg.Server.DBPath)
	if err != nil {
		log.Fatalf("Error connecting to DB: %v", err)
	}
	defer repo.Close()

	// Ensure tables exist
	if err := repo.Migrate("migrations/001_initial_schema.sql"); err != nil {
		log.Fatalf("Error running migrations: %v", err)
	}

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "plugin":
		handlePlugin(repo)
	case "version":
		handleVersion(repo)
	case "license":
		handleLicense(repo)
	case "list":
		handleList(repo)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: update-cli <command> [options]")
	fmt.Println("\nCommands:")
	fmt.Println("  plugin add    Add or update a plugin")
	fmt.Println("  version add   Add a new version for a plugin")
	fmt.Println("  license gen   Generate a license key")
	fmt.Println("  list plugins  List all plugins")
	fmt.Println("  list versions List versions for a plugin")
}

func handlePlugin(repo *repository.SQLiteRepository) {
	pluginCmd := flag.NewFlagSet("plugin", flag.ExitOnError)
	slug := pluginCmd.String("slug", "", "Plugin slug (required)")
	name := pluginCmd.String("name", "", "Plugin display name (required)")
	secret := pluginCmd.String("secret", "", "HMAC secret for licenses (optional)")
	paid := pluginCmd.Bool("paid", false, "Whether it's a paid plugin")

	if len(os.Args) < 3 || os.Args[2] != "add" {
		fmt.Println("Usage: update-cli plugin add --slug <slug> --name <name> [--secret <secret>] [--paid]")
		os.Exit(1)
	}

	pluginCmd.Parse(os.Args[3:])

	if *slug == "" || *name == "" {
		pluginCmd.Usage()
		os.Exit(1)
	}

	err := repo.SavePlugin(repository.Plugin{
		Slug:   *slug,
		Name:   *name,
		Secret: *secret,
		IsPaid: *paid,
	})
	if err != nil {
		log.Fatalf("Failed to save plugin: %v", err)
	}
	fmt.Printf("Successfully saved plugin: %s\n", *slug)
}

func handleVersion(repo *repository.SQLiteRepository) {
	versionCmd := flag.NewFlagSet("version", flag.ExitOnError)
	slug := versionCmd.String("slug", "", "Plugin slug (required)")
	ver := versionCmd.String("version", "", "Version string (e.g. 1.0.0) (required)")
	url := versionCmd.String("url", "", "Download URL (required)")
	changelog := versionCmd.String("changelog", "", "HTML/Markdown changelog")
	reqWP := versionCmd.String("req-wp", "6.0", "Requires WP version")
	testWP := versionCmd.String("test-wp", "6.5", "Tested WP version")
	reqPHP := versionCmd.String("req-php", "7.4", "Requires PHP version")

	if len(os.Args) < 3 || os.Args[2] != "add" {
		fmt.Println("Usage: update-cli version add --slug <slug> --version <v> --url <url> ...")
		os.Exit(1)
	}

	versionCmd.Parse(os.Args[3:])

	if *slug == "" || *ver == "" || *url == "" {
		versionCmd.Usage()
		os.Exit(1)
	}

	err := repo.SaveVersion(repository.Version{
		PluginSlug:  *slug,
		Version:     *ver,
		DownloadURL: *url,
		Changelog:   *changelog,
		RequiresWP:  *reqWP,
		TestedWP:    *testWP,
		RequiresPHP: *reqPHP,
	})
	if err != nil {
		log.Fatalf("Failed to save version: %v", err)
	}
	fmt.Printf("Successfully added version %s for %s\n", *ver, *slug)
}

func handleLicense(repo *repository.SQLiteRepository) {
	licenseCmd := flag.NewFlagSet("license", flag.ExitOnError)
	slug := licenseCmd.String("slug", "", "Plugin slug (required)")
	cid := licenseCmd.String("cid", "", "Client ID (required)")
	days := licenseCmd.Int("days", 365, "Validity in days")

	if len(os.Args) < 3 || os.Args[2] != "gen" {
		fmt.Println("Usage: update-cli license gen --slug <slug> --cid <cid> [--days 365]")
		os.Exit(1)
	}

	licenseCmd.Parse(os.Args[3:])

	if *slug == "" || *cid == "" {
		licenseCmd.Usage()
		os.Exit(1)
	}

	p, err := repo.GetPlugin(*slug)
	if err != nil {
		log.Fatalf("Error fetching plugin: %v", err)
	}
	if p == nil {
		log.Fatalf("Plugin not found: %s", *slug)
	}

	expires := time.Now().AddDate(0, 0, *days)
	key, err := auth.GenerateLicenseKey(p.Slug, *cid, expires, p.Secret)
	if err != nil {
		log.Fatalf("Error generating key: %v", err)
	}

	// Log license creation
	auth.LogLicenseCreated(key, *cid, expires, "cli")

	fmt.Printf("\nLicense Key for %s (Client: %s):\n", *slug, *cid)
	fmt.Printf("Expires: %s\n", expires.Format("2006-01-02"))
	fmt.Printf("\n%s\n\n", key)
}

func handleList(repo *repository.SQLiteRepository) {
	if len(os.Args) < 3 {
		fmt.Println("Usage: update-cli list <plugins|versions> [options]")
		os.Exit(1)
	}

	switch os.Args[2] {
	case "plugins":
		plugins, err := repo.ListPlugins()
		if err != nil {
			log.Fatalf("Error listing plugins: %v", err)
		}
		fmt.Printf("%-20s %-20s %-10s %-5s\n", "SLUG", "NAME", "SECRET", "PAID")
		fmt.Println("------------------------------------------------------------")
		for _, p := range plugins {
			hasSecret := "No"
			if p.Secret != "" {
				hasSecret = "Yes"
			}
			fmt.Printf("%-20s %-20s %-10s %-5t\n", p.Slug, p.Name, hasSecret, p.IsPaid)
		}
	case "versions":
		listVerCmd := flag.NewFlagSet("list versions", flag.ExitOnError)
		slug := listVerCmd.String("slug", "", "Plugin slug (required)")
		listVerCmd.Parse(os.Args[3:])

		if *slug == "" {
			listVerCmd.Usage()
			os.Exit(1)
		}

		versions, err := repo.ListVersions(*slug)
		if err != nil {
			log.Fatalf("Error listing versions: %v", err)
		}
		fmt.Printf("%-10s %-20s %-15s\n", "VERSION", "CREATED AT", "DOWNLOAD URL")
		fmt.Println("------------------------------------------------------------")
		for _, v := range versions {
			fmt.Printf("%-10s %-20s %-15s\n", v.Version, v.CreatedAt, v.DownloadURL)
		}
	default:
		fmt.Println("Usage: update-cli list <plugins|versions> [options]")
		os.Exit(1)
	}
}
