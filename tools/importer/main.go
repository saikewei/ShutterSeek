package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"shutterseek/tools/importer/internal/config"
	"shutterseek/tools/importer/internal/db"
	"shutterseek/tools/importer/internal/importer"
	"shutterseek/tools/importer/internal/scanner"
)

func main() {
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	dryRun := flag.Bool("dry-run", false, "仅扫描打印文件列表，不写入数据库")
	workers := flag.Int("workers", 0, "并发 worker 数（0=使用配置文件值）")
	batchSize := flag.Int("batch", 0, "每批处理文件数（0=使用配置文件值）")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if *workers > 0 {
		cfg.Scanner.Workers = *workers
	}
	if *batchSize > 0 {
		cfg.Scanner.BatchSize = *batchSize
	}

	log.Printf("ShutterSeek 照片元数据提取入库")
	log.Printf("  目录: %s  数据库: %s:%d/%s",
		cfg.Scanner.PhotosDir, cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName)
	log.Printf("  并发: %d workers × %d batch",
		cfg.Scanner.Workers, cfg.Scanner.BatchSize)

	if *dryRun {
		runDryRun(cfg)
		return
	}

	// --- Real import ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect DB
	pool, err := db.NewPool(ctx, cfg.Database.DSN(), cfg.Database.MaxConnections)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	defer pool.Close()
	log.Println("✓ 数据库连接成功")

	// Handle Ctrl+C gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("\n收到退出信号，等待当前批次完成...")
		cancel()
	}()

	// Start scanner
	paths := make(chan string, cfg.Scanner.BatchSize*10)
	go func() {
		if err := scanner.Walk(cfg.Scanner.PhotosDir, cfg.Scanner.SkipVideo, paths); err != nil {
			log.Printf("扫描警告: %v", err)
		}
	}()

	// Start importer
	imp := importer.New(pool, cfg.Scanner.Workers, cfg.Scanner.BatchSize, cfg.Scanner.PhotosDir)
	start := time.Now()

	// Progress reporter
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				elapsed := time.Since(start).Round(time.Second)
				s := imp.Stats()
				rate := float64(s.Total) / elapsed.Seconds()
				log.Printf("进度: %s | %v | %.0f/s", s.String(), elapsed, rate)
			}
		}
	}()

	stats := imp.Run(ctx, paths)

	elapsed := time.Since(start).Round(time.Second)
	log.Printf("========== 完成 ==========")
	log.Printf("  耗时:     %v", elapsed)
	log.Printf("  统计:     %s", stats.String())
	rate := float64(stats.Total) / elapsed.Seconds()
	log.Printf("  速率:     %.0f 文件/秒", rate)
}

func runDryRun(cfg *config.Config) {
	paths := make(chan string, 1000)
	go func() {
		scanner.Walk(cfg.Scanner.PhotosDir, cfg.Scanner.SkipVideo, paths)
	}()

	count := 0
	for p := range paths {
		count++
		if count%500 == 0 {
			fmt.Printf("  [%d] %s\n", count, p)
		}
	}
	fmt.Printf("\n扫描完成: 共 %d 个文件\n", count)
}
