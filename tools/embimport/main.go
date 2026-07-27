package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	binPath := "/tmp/embeddings/embeddings.bin"
	idsPath := "/tmp/embeddings/image_ids.txt"
	dsn := "postgres://photo_user:PhotoHyc65319436@172.18.0.2:5432/photo_search?sslmode=disable"

	log.Println("Reading binary vectors...")
	data, err := os.ReadFile(binPath)
	if err != nil {
		log.Fatalf("read embeddings.bin: %v", err)
	}

	n := int(binary.LittleEndian.Uint32(data[0:4]))
	dim := int(binary.LittleEndian.Uint32(data[4:8]))
	log.Printf("Vectors: %d × %d dimensions", n, dim)

	// Parse float32 vectors (little-endian)
	byteSlice := data[8:]
	floats := make([]float32, n*dim)
	for i := range floats {
		floats[i] = math.Float32frombits(
			binary.LittleEndian.Uint32(byteSlice[i*4 : i*4+4]),
		)
	}
	log.Printf("Parsed %d float32 values (%.1f MB)", len(floats), float64(len(floats)*4)/(1024*1024))

	// Read image IDs
	log.Println("Reading image IDs...")
	f, err := os.Open(idsPath)
	if err != nil {
		log.Fatalf("open image_ids.txt: %v", err)
	}
	defer f.Close()

	ids := make([]int64, 0, n)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// "38360.jpg" → 38360
		idStr := strings.TrimSuffix(line, ".jpg")
		var id int64
		fmt.Sscanf(idStr, "%d", &id)
		ids = append(ids, id)
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("scan image_ids.txt: %v", err)
	}
	log.Printf("Image IDs: %d", len(ids))

	if len(ids) != n {
		log.Fatalf("mismatch: %d vectors but %d IDs", n, len(ids))
	}

	// Connect to DB
	log.Println("Connecting to database...")
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	// Check photo count and verify IDs exist
	var photoCount int
	pool.QueryRow(context.Background(), "SELECT count(*) FROM photos").Scan(&photoCount)
	log.Printf("Photos in DB: %d", photoCount)

	// Build vectors and import via COPY
	log.Println("Importing embeddings (COPY)...")
	start := time.Now()

	tx, err := pool.Begin(context.Background())
	if err != nil {
		log.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(context.Background())

	// Batch INSERT with string-formatted vectors
	batchSize := 500
	for i := 0; i < n; i += batchSize {
		end := i + batchSize
		if end > n {
			end = n
		}

		// Build multi-row INSERT
		b := &strings.Builder{}
		b.WriteString("INSERT INTO photo_embeddings (photo_id, embedding) VALUES ")
		args := make([]any, 0, (end-i)*2)

		for j := i; j < end; j++ {
			if j > i {
				b.WriteString(", ")
			}
			// Format vector as pgvector string literal
			vecStart := j * dim
			vecStr := formatVector(floats[vecStart : vecStart+dim])
			b.WriteString(fmt.Sprintf("($%d, $%d::vector)", j*2-i*2+1, j*2-i*2+2))
			args = append(args, ids[j], vecStr)
		}

		_, err := tx.Exec(context.Background(), b.String(), args...)
		if err != nil {
			log.Fatalf("insert batch %d-%d: %v", i, end, err)
		}

		if i%10000 == 0 {
			log.Printf("  inserted %d/%d (%.1f%%)", i, n, float64(i)/float64(n)*100)
		}
	}
	log.Printf("  inserted %d/%d (100%%)", n, n)

	if err := tx.Commit(context.Background()); err != nil {
		log.Fatalf("commit: %v", err)
	}

	elapsed := time.Since(start).Round(time.Second)
	log.Printf("Done! %d vectors imported in %v (%.0f vectors/sec)",
		n, elapsed, float64(n)/elapsed.Seconds())
}

// formatVector formats a float32 slice as pgvector string: [0.1,0.2,...]
func formatVector(v []float32) string {
	b := &strings.Builder{}
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(b, "%g", f)
	}
	b.WriteByte(']')
	return b.String()
}
