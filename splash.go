package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

var httpClient = &http.Client{}

// Flags
var (
	platform    string
	manifestID  string
	installPath string
	cachePath   string
	cloudURL    string
)

func init() {
	// Parse flags
	flag.StringVar(&platform, "platform", "Windows", "platform to download for")
	flag.StringVar(&manifestID, "manifest", "", "download a specific manifest")
	flag.StringVar(&installPath, "installdir", "files", "install path")
	flag.StringVar(&cachePath, "cache", "cache", "cache path")
	flag.StringVar(&cloudURL, "cloud", "https://epicgames-download1.akamaized.net/Builds/Fortnite/CloudDir/", "cloud url")
	flag.Parse()
}

func main() {
	// Make working directories
	os.Mkdir(cachePath, os.ModePerm)
	os.Mkdir(installPath, os.ModePerm)

	var catalog *Catalog
	var manifest *Manifest

	// Load catalog
	catalogCachePath := filepath.Join(cachePath, "catalog.json")
	if _, err := os.Stat(catalogCachePath); err == nil { // read catalog from cache
		log.Println("Loading catalog from cache...")

		// Read from disk
		catalog, err = readCatalogFile(catalogCachePath)
		if err != nil {
			log.Fatalf("Failed to load catalog: %v", err)
		}
	} else { // otherwise, fetch latest
		log.Println("Fetching latest catalog...")

		// Fetch from MCP
		catalogBytes, err := fetchCatalog(platform, "fn", "4fe75bbc5a674f4f9b356b5c90567da5", "Fortnite", "Live")
		if err != nil {
			log.Fatalf("Failed to fetch catalog: %v", err)
		}

		// Parse data
		catalog, err = parseCatalog(catalogBytes)
		if err != nil {
			log.Fatalf("Failed to parse catalog: %v", err)
		}

		// Save to cache
		ioutil.WriteFile(catalogCachePath, catalogBytes, 0644)
	}

	// Sanity check catalog
	if len(catalog.Elements) != 1 || len(catalog.Elements[0].Manifests) < 1 {
		log.Fatal("Unsupported catalog")
	}

	log.Printf("Catalog %s (%s) %s loaded.\n", catalog.Elements[0].AppName, catalog.Elements[0].LabelName, catalog.Elements[0].BuildVersion)

	// Load manifest
	manifestCachePath := filepath.Join(cachePath, "manifest.json")
	if manifestID != "" { // fetch specific manifest
		log.Printf("Fetching manifest %s...", manifestID)

		var err error
		manifest, _, err = fetchManifest(cloudURL + manifestID + ".manifest")
		if err != nil {
			log.Fatalf("Failed to fetch manifest: %v", err)
		}
	} else if _, err := os.Stat(manifestCachePath); err == nil { // read manifest from disk
		log.Println("Loading manifest from cache...")

		manifest, err = readManifestFile(manifestCachePath)
		if err != nil {
			log.Fatalf("Failed to read manifest: %v", err)
		}
	} else { // otherwise, fetch from web
		log.Println("Fetching latest manifest...")

		var manifestBytes []byte
		manifest, manifestBytes, err = fetchManifest(catalog.GetManifestURL())
		if err != nil {
			log.Fatalf("Failed to fetch manifest: %v", err)
		}
		ioutil.WriteFile(manifestCachePath, manifestBytes, 0644)
	}

	log.Printf("Manifest %s %s loaded.\n", manifest.AppNameString, manifest.BuildVersionString)
}
