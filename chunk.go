package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"strconv"
	"strings"
)

// Chunk defines a downloadable chunk
type Chunk struct {
	GUID      string
	Hash      string
	Sha       string
	DataGroup int
	FileSize  int64
}

// ChunkPart defines a part of a specific chunk
type ChunkPart struct {
	Offset uint32
	Size   uint32
}

// ChunkJob defines a job
type ChunkJob struct {
	ID    int
	Chunk Chunk
	Part  ChunkPart
}

// ChunkJobResult defines a result
type ChunkJobResult struct {
	Job    ChunkJob
	Reader io.ReadSeeker
}

// ChunkHeader defines the binary chunk header
type ChunkHeader struct {
	Magic              uint32 // 0xB1FE3AA2
	Version            uint32 // 2
	HeaderSize         uint32 // 3E
	DataSizeCompressed uint32
	GUID               [16]byte
	RollingHash        uint64
	StoredAs           uint8 // 00 = plaintext, 01 = compressed, 02 = encrypted
	SHAHash            [20]byte
	HashType           uint8 // strangely 03
}

// GetURL builds a url
func (c *Chunk) GetURL(cloudURL string) string {
	return fmt.Sprintf("%s/Builds/Fortnite/CloudDir/ChunksV3/%02d/%s_%s.chunk", cloudURL, c.DataGroup, c.Hash, c.GUID)
}

// Download fetches the chunk from the internet
func (c *Chunk) Download(cloudURL string) (data []byte, err error) {
	// Make GET request
	resp, err := httpClient.Get(c.GetURL(cloudURL))
	if err != nil {
		return
	}
	defer resp.Body.Close()

	// Check response code
	if resp.StatusCode != 200 {
		err = fmt.Errorf("invalid status code %d", resp.StatusCode)
		return
	}

	// Read data
	data, err = ioutil.ReadAll(resp.Body)

	return
}

// NewChunk create a chunk object
func NewChunk(guid string, hash string, sha string, dataGroup string, fileSize string) Chunk {
	dg, err := strconv.Atoi(dataGroup)
	if err != nil {
		log.Fatalf("Failed to convert datagroup %s: %v", dataGroup, err)
	}

	parsedHash := readPackedData(hash)
	reverse(parsedHash)

	return Chunk{
		GUID:      guid,
		Hash:      strings.ToUpper(hex.EncodeToString(parsedHash)),
		Sha:       sha,
		DataGroup: dg,
		FileSize:  int64(readPackedUint32(fileSize)),
	}
}

func readChunkHeader(r io.ReadSeeker) (ChunkHeader, error) {
	// Initialize empty header
	header := ChunkHeader{}

	// Read header
	err := binary.Read(r, binary.LittleEndian, &header)

	return header, err
}

func readPackedData(packed string) []byte {
	output := make([]byte, 0)

	for i := 0; i < len(packed); i += 3 {
		num, err := strconv.ParseUint(packed[i:i+3], 10, 16)
		if err != nil {
			return nil
		}

		output = append(output, byte(num))
	}

	return output
}

func readPackedUint32(packed string) uint32 {
	return binary.LittleEndian.Uint32(readPackedData(packed))
}
