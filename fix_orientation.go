package main

import (
	"context"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	pool, _ := pgxpool.New(context.Background(),
		"postgres://photo_user:PhotoHyc65319436@172.18.0.2:5432/photo_search?sslmode=disable")
	defer pool.Close()

	rows, _ := pool.Query(context.Background(),
		`SELECT id, file_path, width, height FROM photos
		 WHERE file_path ~* '\.(arw|nef|rw2|dng|cr2|cr3|orf|raf|pef|sr2|srf)$'
		 AND taken_at IS NOT NULL ORDER BY id`)

	type rec struct{ id int64; path string; w, h int32 }
	var recs []rec
	for rows.Next() {
		var r rec
		rows.Scan(&r.id, &r.path, &r.w, &r.h)
		recs = append(recs, r)
	}
	rows.Close()
	log.Printf("%d RAW files...", len(recs))

	start := time.Now()
	var swapped, checked int

	for i := 0; i < len(recs); i += 100 {
		end := i + 100
		if end > len(recs) { end = len(recs) }
		batch := recs[i:end]
		args := []string{"-Orientation", "-b"}
		for _, r := range batch { args = append(args, "/photos/"+r.path) }
		out, _ := exec.Command("exiftool", args...).Output()
		for j, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if i+j >= len(recs) { break }
			checked++
			r := recs[i+j]
			if strings.Contains(line, "Rotate 90") || strings.Contains(line, "Rotate 270") {
				pool.Exec(context.Background(), "UPDATE photos SET width=$1, height=$2 WHERE id=$3", r.h, r.w, r.id)
				swapped++
			}
		}
		if i%5000 == 0 { log.Printf("%d/%d %d swapped %v", checked, len(recs), swapped, time.Since(start).Round(time.Second)) }
	}
	log.Printf("Done: %d checked, %d swapped, %v", checked, swapped, time.Since(start).Round(time.Second))
}
