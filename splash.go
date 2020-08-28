package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var httpClient = &http.Client{}
var chunkCache = make(map[string][]byte)
var chunkParentCount = make(map[string]int)
var cacheLock sync.Mutex

// Flags
var (
	platform           string
	manifestID         string
	installPath        string
	cachePath          string
	fileFilter         string
	downloadURLs       []string
	skipIntegrityCheck bool
	workerCount        int
)

const defaultDownloadURL = "http://epicgames-download1.akamaized.net"

func init() {
	// Seed random
	rand.Seed(time.Now().Unix())

	// Parse flags
	flag.StringVar(&platform, "platform", "Windows", "platform to download for")
	flag.StringVar(&manifestID, "manifest", "", "download a specific manifest")
	flag.StringVar(&installPath, "install-dir", "files", "folder to write downloaded files to")
	flag.StringVar(&cachePath, "cache", "", "path to folder for caching manifest and catalog")
	flag.StringVar(&fileFilter, "files", "", "comma-separated list of files to download")
	dlUrls := flag.String("url", defaultDownloadURL, "download url")
	flag.BoolVar(&skipIntegrityCheck, "skipcheck", false, "skip file integrity check")
	flag.IntVar(&workerCount, "workers", 10, "amount of workers")
	flag.Parse()

	downloadURLs = strings.Split(*dlUrls, ",")

	// Set http timeout
	httpClient.Timeout = 30 * time.Second
}

func main() {
	// Make working directories
	if cachePath != "" {
		os.MkdirAll(cachePath, os.ModePerm)
	}

	var catalog *Catalog
	var manifest *Manifest

	// Load catalog
	if manifestID == "" {
		catalogCachePath := filepath.Join(cachePath, "catalog.json")
		if _, err := os.Stat(catalogCachePath); err == nil && cachePath != "" { // read catalog from cache
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
			if cachePath != "" {
				ioutil.WriteFile(catalogCachePath, catalogBytes, 0644)
			}
		}

		// Sanity check catalog
		if len(catalog.Elements) != 1 || len(catalog.Elements[0].Manifests) < 1 {
			log.Fatal("Unsupported catalog")
		}

		log.Printf("Catalog %s (%s) %s loaded.\n", catalog.Elements[0].AppName, catalog.Elements[0].LabelName, catalog.Elements[0].BuildVersion)
	}

	// Load manifest
	manifestCachePath := filepath.Join(cachePath, "manifest.json")
	if manifestID != "" { // fetch specific manifest
		log.Printf("Fetching manifest %s...", manifestID)

		var err error
		manifest, _, err = fetchManifest(fmt.Sprintf("%s/Builds/Fortnite/CloudDir/%s.manifest", defaultDownloadURL, manifestID))
		if err != nil {
			log.Fatalf("Failed to fetch manifest: %v", err)
		}
	} else if _, err := os.Stat(manifestCachePath); err == nil && cachePath != "" { // read manifest from disk
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
		if cachePath != "" {
			ioutil.WriteFile(manifestCachePath, manifestBytes, 0644)
		}
	}

	log.Printf("Manifest %s %s loaded.\n", manifest.AppNameString, manifest.BuildVersionString)

	manifestFiles := make(map[string]ManifestFile)
	manifestChunks := make(map[string]Chunk)
	checkedFiles := make(map[string]ManifestFile)

	// Parse manifest
	for _, file := range manifest.FileManifestList {
		// Add file
		manifestFiles[file.FileName] = file

		// Add all chunks
		for _, c := range file.FileChunkParts {
			chunkParentCount[c.GUID]++

			if _, ok := manifestChunks[c.GUID]; !ok { // don't add duplicates
				manifestChunks[c.GUID] = NewChunk(c.GUID, manifest.ChunkHashList[c.GUID], manifest.ChunkShaList[c.GUID], manifest.DataGroupList[c.GUID], manifest.ChunkFilesizeList[c.GUID])
			}
		}
	}

	log.Printf("Found %d files and %d chunks in manifest.\n", len(manifestFiles), len(manifestChunks))

	// File filter
	if fileFilter != "" {
		tempFiles := make(map[string]ManifestFile)
		for _, fileName := range strings.Split(fileFilter, ",") {
			if f, ok := manifestFiles[fileName]; ok {
				tempFiles[fileName] = f
			}
		}
		manifestFiles = tempFiles
	}

	// Download and assemble files
	for k, file := range manifestFiles {
		func() {
			filePath := filepath.Join(installPath, file.FileName)

			// Check if file already exists
			if _, err := os.Stat(filePath); err == nil {
				// Open file
				diskFile, err := os.Open(filePath)
				if err == nil {
					// Calculate checksum
					hasher := sha1.New()
					_, err := io.Copy(hasher, diskFile)
					diskFile.Close()

					if err == nil {
						// Compare checksum
						if bytes.Equal(hasher.Sum(nil), readPackedData(file.FileHash)) {
							// Remove any trailing chunks
							for _, chunkPart := range file.FileChunkParts {
								chunkUsed(chunkPart.GUID)
							}

							log.Printf("File %s found on disk!\n", file.FileName)
							checkedFiles[k] = file
							return
						}
					}
				}
			}

			log.Printf("Downloading %s from %d chunks...\n", file.FileName, len(file.FileChunkParts))

			// Create outfile
			os.MkdirAll(filepath.Dir(filePath), os.ModePerm)
			outFile, err := os.Create(filePath)
			if err != nil {
				log.Printf("Failed to create %s: %v\n", filePath, err)
				return
			}
			defer outFile.Close()

			// Parse chunk parts
			chunkPartCount := len(file.FileChunkParts)
			chunkJobs := make([]ChunkJob, chunkPartCount)
			jobs := make(chan ChunkJob, chunkPartCount)
			for i, chunkPart := range file.FileChunkParts {
				chunkJobs[i] = ChunkJob{ID: i, Chunk: manifestChunks[chunkPart.GUID], Part: ChunkPart{Offset: readPackedUint32(chunkPart.Offset), Size: readPackedUint32(chunkPart.Size)}}
				jobs <- chunkJobs[i]
			}

			results := make(chan ChunkJobResult, chunkPartCount)
			orderedResults := make(chan ChunkJobResult, chunkPartCount)

			// Order results as they come in
			go func() {
				resultsBuffer := make(map[int]ChunkJobResult)
				for result := range results {
					resultsBuffer[result.Job.ID] = result

				loop:
					if len(chunkJobs) > 0 {
						if res, ok := resultsBuffer[chunkJobs[0].ID]; ok {
							orderedResults <- res
							chunkJobs = chunkJobs[1:]
							delete(resultsBuffer, res.Job.ID)
							goto loop
						}
					}
				}
			}()

			// Spawn workers
			for i := 0; i < workerCount; i++ {
				go chunkWorker(jobs, results)
			}

			// Handle results
			for i := 0; i < chunkPartCount; i++ {
				result := <-orderedResults

				// Write chunk part to file
				result.Reader.Seek(int64(result.Job.Part.Offset), io.SeekCurrent)
				_, err := io.CopyN(outFile, result.Reader, int64(result.Job.Part.Size))
				if err != nil {
					log.Printf("Failed to write chunk %s to file %s: %v\n", result.Job.Chunk.GUID, file.FileName, err)
					continue
				}
			}
			close(jobs)
			close(results)
		}()
	}

	// Integrity check
	if !skipIntegrityCheck {
		log.Println("Verifying file integrity...")

		for k, file := range manifestFiles {
			// Skip prechecked files
			if _, ok := checkedFiles[k]; ok {
				continue
			}

			filePath := filepath.Join(installPath, file.FileName)

			// Open file
			f, err := os.Open(filePath)
			if err != nil {
				log.Printf("Failed to open %s: %v\n", file.FileName, err)
				continue
			}

			// Hash file
			hasher := sha1.New()
			_, err = io.Copy(hasher, f)
			f.Close()

			if err != nil {
				log.Printf("Failed to hash %s: %v\n", file.FileName, err)
				continue
			}

			// Compare checksum
			expectedHash := readPackedData(file.FileHash)
			actualHash := hasher.Sum(nil)
			if !bytes.Equal(actualHash, expectedHash) {
				log.Printf("File %s is corrupt - got hash %s but want %s\n", file.FileName, hex.EncodeToString(actualHash), hex.EncodeToString(expectedHash))
			}
		}
	}

	log.Println("Done!")
}

func chunkUsed(guid string) {
	// Chunk was used once
	chunkParentCount[guid]--

	// Check if we still need to store chunk in cache
	if chunkParentCount[guid] < 1 {
		delete(chunkCache, guid)
	}
}

func chunkWorker(jobs chan ChunkJob, results chan<- ChunkJobResult) {
	for j := range jobs {
		var chunkReader io.ReadSeeker
		cacheLock.Lock()
		if _, ok := chunkCache[j.Chunk.GUID]; ok {
			// Read from cache
			chunkReader = bytes.NewReader(chunkCache[j.Chunk.GUID])

			cacheLock.Unlock()
		} else {
			cacheLock.Unlock()

			// Download chunk
			chunkData, err := j.Chunk.Download(downloadURLs[rand.Intn(len(downloadURLs))])
			if err != nil {
				log.Printf("Failed to download chunk %s: %v\n", j.Chunk.GUID, err)
				jobs <- j // requeue
				continue
			}

			// Create new reader
			chunkReader = bytes.NewReader(chunkData)

			// Read chunk header
			chunkHeader, err := readChunkHeader(chunkReader)
			if err != nil {
				log.Printf("Failed to read chunk header %s: %v\n", j.Chunk.GUID, err)
				jobs <- j
				continue
			}

			// Decompress if needed
			if chunkHeader.StoredAs == 1 {
				// Create decompressor
				zlibReader, err := zlib.NewReader(chunkReader)
				if err != nil {
					log.Printf("Failed to create decompressor for chunk %s: %v\n", j.Chunk.GUID, err)
					jobs <- j
					continue
				}

				// Decompress entire chunk
				chunkData, err = ioutil.ReadAll(zlibReader)
				if err != nil {
					log.Printf("Failed to decompress chunk %s: %v\n", j.Chunk.GUID, err)
					jobs <- j
					continue
				}

				// Set reader to decompressed data
				chunkReader = bytes.NewReader(chunkData)
			} else if chunkHeader.StoredAs != 0 {
				log.Printf("Got unknown chunk (storedas: %d) %s\n", chunkHeader.StoredAs, j.Chunk.GUID)
				jobs <- j
				continue
			}

			// Store in cache if needed later
			cacheLock.Lock()
			if chunkParentCount[j.Chunk.GUID] > 1 {
				if chunkHeader.StoredAs == 0 { // chunkData still contains header here
					chunkCache[j.Chunk.GUID] = chunkData[62:]
				} else {
					chunkCache[j.Chunk.GUID] = chunkData
				}
			}
			cacheLock.Unlock()
		}

		// Chunk was used once
		cacheLock.Lock()
		chunkUsed(j.Chunk.GUID)
		cacheLock.Unlock()

		// Pass result
		results <- ChunkJobResult{Job: j, Reader: chunkReader}
	}
}
