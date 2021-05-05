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
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
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
	manifestPath       string
	installPath        string
	chunkPath          string
	onlyDLChunks       bool
	fileFilter         map[string]bool = make(map[string]bool)
	downloadURLs       []string
	skipIntegrityCheck bool
	workerCount        int
	killSignal         bool = false
)

const defaultDownloadURL = "http://epicgames-download1.akamaized.net"

func init() {
	// Seed random
	rand.Seed(time.Now().Unix())

	// Parse flags
	flag.StringVar(&platform, "platform", "Windows", "platform to download for")
	flag.StringVar(&manifestID, "manifest", "", "download specific manifest(s)")
	flag.StringVar(&manifestPath, "manifest-file", "", "download specific manifest(s) - comma-separated list")
	flag.StringVar(&installPath, "install-dir", "", "folder to write downloaded files to")
	flag.StringVar(&chunkPath, "chunk-dir", "", "folder to read predownloaded chunks from")
	flag.BoolVar(&onlyDLChunks, "chunks-only", false, "only download chunks")
	dlFilter := flag.String("files", "", "comma-separated list of files to download")
	dlUrls := flag.String("url", defaultDownloadURL, "download url")
	httpTimeout := flag.Int64("http-timeout", 60, "http timeout in seconds")
	flag.BoolVar(&skipIntegrityCheck, "skipcheck", false, "skip file integrity check")
	flag.IntVar(&workerCount, "workers", 10, "amount of workers")
	flag.Parse()

	if manifestPath == "" {
		manifestPath = flag.Arg(0)
	}

	for _, file := range strings.Split(*dlFilter, ",") {
		if file != "" {
			fileFilter[file] = true
		}
	}

	downloadURLs = strings.Split(*dlUrls, ",")
	httpClient.Timeout = time.Duration(*httpTimeout) * time.Second
}

func main() {
	var catalog *Catalog
	manifests := make([]*Manifest, 0)

	// Load catalog
	if manifestID == "" && manifestPath == "" {
		// Fetch latest catalog
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

		// Sanity check catalog
		if len(catalog.Elements) != 1 || len(catalog.Elements[0].Manifests) < 1 {
			log.Fatal("Unsupported catalog")
		}

		log.Printf("Catalog %s (%s) %s loaded.\n", catalog.Elements[0].AppName, catalog.Elements[0].LabelName, catalog.Elements[0].BuildVersion)
	}

	// Load manifest
	if manifestID != "" { // fetch specific manifest(s)
		for _, id := range strings.Split(manifestID, ",") {
			log.Printf("Fetching manifest %s...", id)

			manifest, _, err := fetchManifest(fmt.Sprintf("https://github.com/VastBlast/FortniteManifestArchive/raw/main/Fortnite/Windows/%s.manifest", id))
			if err != nil {
				log.Fatalf("Failed to fetch manifest: %v", err)
			}
			manifests = append(manifests, manifest)
		}
	} else if manifestPath != "" { // read manifest(s) from disk
		for _, manifestPath := range strings.Split(manifestPath, ",") {
			// Check if folder
			if fi, err := os.Stat(manifestPath); err == nil && fi.IsDir() {
				loaded := 0

				// Walk folder
				if err := filepath.Walk(manifestPath, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return fmt.Errorf("failed to walk: %v", err)
					}

					if info.IsDir() || info.Size() == 0 {
						return nil
					}

					// Read manifest
					manifest, err := readManifestFile(path)
					if err != nil {
						log.Fatalf("Failed to read manifest from folder: %v", err)
					}
					manifests = append(manifests, manifest)
					loaded++

					return nil
				}); err != nil {
					log.Fatalf("Failed to read manifests from folder: %v", err)
				}

				log.Printf("Loaded %d manifests from %s.\n", loaded, manifestPath)
				continue
			}

			manifest, err := readManifestFile(manifestPath)
			if err != nil {
				log.Fatalf("Failed to read manifest %s: %v", manifestPath, err)
			}

			log.Printf("Manifest %s %s loaded.\n", manifest.AppNameString, manifest.BuildVersionString)

			manifests = append(manifests, manifest)
		}
	} else { // otherwise, fetch from catalog
		log.Println("Fetching latest manifest...")

		manifest, _, err := fetchManifest(catalog.GetManifestURL())
		if err != nil {
			log.Fatalf("Failed to fetch manifest: %v", err)
		}
		manifests = append(manifests, manifest)
	}

	manifestFiles := make(map[string]ManifestFile)
	manifestChunks := make(map[string]Chunk)
	checkedFiles := make(map[string]ManifestFile)

	// Parse manifests
	for _, manifest := range manifests {
		for _, file := range manifest.FileManifestList {
			// Check filter
			if _, ok := fileFilter[file.FileName]; !ok && len(fileFilter) > 0 {
				continue
			}

			// Set full file path
			file.FileName = filepath.Join(installPath, strings.TrimSuffix(strings.TrimPrefix(manifest.BuildVersionString, "++Fortnite+Release-"), "-"+platform), file.FileName)

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
	}

	// Setup interrupt handler
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("Shutting down...")
		killSignal = true
	}()

	// Handle chunk-only download
	if onlyDLChunks {
		log.Printf("Downloading %d chunks...\n", len(manifestChunks))

		// Build job queue
		jobs := make(chan Chunk, len(manifestChunks))
		for _, chunk := range manifestChunks {
			jobs <- chunk
		}
		close(jobs)

		// Workers
		var wg sync.WaitGroup
		for i := 0; i < workerCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := range jobs {
					if killSignal {
						return
					}

					filePath := filepath.Join(chunkPath, j.GUID)

					// Check if present on disk
					if fi, err := os.Stat(filePath); err == nil && fi.Size() == j.FileSize {
						continue
					}

					// Download chunk
					chunkData, err := j.Download(downloadURLs[rand.Intn(len(downloadURLs))])
					if err != nil {
						log.Printf("Failed to download chunk %s: %v\n", j.GUID, err)
						jobs <- j // requeue
						continue
					}

					// Write to disk
					if err := ioutil.WriteFile(filePath, chunkData, 0644); err != nil {
						log.Printf("Failed to write chunk %s: %v\n", j.GUID, err)
						jobs <- j
					}
				}
			}()
		}

		// Wait for all goroutines
		wg.Wait()

		log.Println("Done!")
		os.Exit(0)
	}

	log.Printf("Downloading %d files in %d chunks from %d manifests.\n", len(manifestFiles), len(manifestChunks), len(manifests))

	// Download and assemble files
	for k, file := range manifestFiles {
		if killSignal {
			return
		}

		func() {
			filePath := file.FileName

			// Check if file already exists
			if f, err := os.Open(filePath); err == nil {
				// Compare checksum
				equal, err := checkFile(f, file)
				f.Close()
				if err == nil && equal {
					// Remove any trailing chunks
					for _, chunkPart := range file.FileChunkParts {
						chunkUsed(chunkPart.GUID)
					}

					log.Printf("File %s found on disk!\n", file.FileName)
					checkedFiles[k] = file
					return
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

				// Close reader
				result.Reader.Close()

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

			// Open file
			f, err := os.Open(file.FileName)
			if err != nil {
				log.Printf("Failed to open %s: %v\n", file.FileName, err)
				continue
			}

			// Hash file
			equal, err := checkFile(f, file)
			f.Close()

			if err != nil {
				log.Printf("Failed to hash %s: %v\n", file.FileName, err)
				continue
			}

			if !equal {
				log.Printf("File %s is corrupt\n", file.FileName)
			}
		}
	}

	log.Println("Done!")
}

func checkFile(f *os.File, file ManifestFile) (bool, error) {
	// Parse expected hash
	var hash []byte
	if len(file.FileHash) == 40 {
		hash, _ = hex.DecodeString(file.FileHash)
	} else {
		hash = readPackedData(file.FileHash)
	}

	// Calculate file size
	var totalSize uint32 = 0
	for _, chunk := range file.FileChunkParts {
		totalSize += readPackedUint32(chunk.Size)
	}

	// Compare actual size
	fi, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("failed to stat: %v", err)
	}
	if totalSize != uint32(fi.Size()) {
		return false, nil
	}

	// Calculate checksum
	hasher := sha1.New()
	_, err = io.Copy(hasher, f)

	// Compare checksum
	return bytes.Equal(hasher.Sum(nil), hash), err
}

func chunkUsed(guid string) {
	// Chunk was used once
	chunkParentCount[guid]--

	// Check if we still need to store chunk in cache
	if chunkParentCount[guid] < 1 {
		delete(chunkCache, guid)
	}
}

func parseChunk(reader ReadSeekCloser) (ReadSeekCloser, []byte, error) {
	// Read chunk header
	chunkHeader, err := readChunkHeader(reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read header: %v", err)
	}

	// Decompress if needed
	if chunkHeader.StoredAs == 0 {
		return reader, nil, nil
	} else if chunkHeader.StoredAs == 1 {
		// Create decompressor
		zlibReader, err := zlib.NewReader(reader)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create decompressor: %v", err)
		}

		// Decompress entire chunk
		chunkData, err := ioutil.ReadAll(zlibReader)
		zlibReader.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decompress: %v", err)
		}

		// Set reader to decompressed data
		return NewByteCloser(chunkData), chunkData, nil
	}

	return nil, nil, fmt.Errorf("got unknown chunk: %d", chunkHeader.StoredAs)
}

func chunkWorker(jobs chan ChunkJob, results chan<- ChunkJobResult) {
	for j := range jobs {
		var chunkReader ReadSeekCloser
		cacheLock.Lock()
		_, ok := chunkCache[j.Chunk.GUID]
		cacheLock.Unlock()
		if ok {
			// Read from cache
			chunkReader = NewByteCloser(chunkCache[j.Chunk.GUID])
		} else if rawChunkReader, err := os.Open(filepath.Join(chunkPath, j.Chunk.GUID)); err == nil {
			if err != nil {
				log.Printf("Failed to open chunk %s from disk: %v\n", j.Chunk.GUID, err)
				jobs <- j
				continue
			}

			// Parse chunk
			var decompressedData []byte
			chunkReader, decompressedData, err = parseChunk(rawChunkReader)

			// Close original file reader if we got decompressed data
			if len(decompressedData) > 0 || err != nil {
				rawChunkReader.Close()
			}

			if err != nil {
				log.Printf("Failed to parse chunk %s from disk: %v\n", j.Chunk.GUID, err)
				jobs <- j
				continue
			}
		} else {
			// Download chunk
			rawChunkData, err := j.Chunk.Download(downloadURLs[rand.Intn(len(downloadURLs))])
			if err != nil {
				log.Printf("Failed to download chunk %s: %v\n", j.Chunk.GUID, err)
				jobs <- j // requeue
				continue
			}

			// Create new reader
			chunkReader = NewByteCloser(rawChunkData)

			// Parse chunk
			var chunkData []byte
			chunkReader, chunkData, err = parseChunk(chunkReader)
			if err != nil {
				log.Printf("Failed to parse chunk %s: %v\n", j.Chunk.GUID, err)
				jobs <- j
				continue
			}

			// Store in cache if needed later
			cacheLock.Lock()
			if chunkParentCount[j.Chunk.GUID] > 1 {
				if len(chunkData) > 0 {
					chunkCache[j.Chunk.GUID] = chunkData
				} else {
					chunkCache[j.Chunk.GUID] = rawChunkData[62:] // chunkData still contains header here
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
