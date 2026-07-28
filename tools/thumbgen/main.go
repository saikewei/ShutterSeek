package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"shutterseek/tools/thumbgen/internal/config"
	"shutterseek/tools/thumbgen/internal/db"
	"shutterseek/tools/thumbgen/internal/thumbnail"
)

func main() {
	configPath := flag.String("config", "config.yaml", "config path")
	limit := flag.Int("limit", 20, "number of photos to process (0=all)")
	startID := flag.Int64("start-id", 0, "start from this photo ID")
	workers := flag.Int("workers", 4, "concurrent workers")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg.Database.DSN(), cfg.Database.MaxConnections)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()
	log.Println("✓ DB connected")

	gen := thumbnail.New(pool, cfg.Thumbnail.OutputDir, cfg.Thumbnail.Size, cfg.Scanner.PhotosDir, *workers)

	log.Printf("Processing %d photos...", *limit)
	start := time.Now()
	stats := gen.Run(*limit, *startID)
	elapsed := time.Since(start).Round(time.Millisecond)

	fmt.Println()
	fmt.Printf("=== Results ===\n")
	fmt.Printf("  Total:     %d\n", stats.Total)
	fmt.Printf("  Generated: %d (resized)\n", stats.Generated)
	fmt.Printf("  Small:     %d (kept as-is)\n", stats.SmallKept)
	fmt.Printf("  Skipped:   %d (already exists)\n", stats.Skipped)
	fmt.Printf("  Errors:    %d\n", stats.Errors)
	fmt.Printf("  Duration:  %v\n", elapsed)
	if stats.Total > 0 {
		perFile := elapsed / time.Duration(stats.Total)
		fmt.Printf("  Per file:  %v\n", perFile)

		// Estimate for all 75400 photos
		estTotal := perFile * 75400
		fmt.Printf("\n  ≈ Estimate for 75,400 photos: %v\n", estTotal.Round(time.Second))
	}
}
