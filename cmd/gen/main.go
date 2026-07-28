package main

import (
	"fmt"
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gen"
	"gorm.io/gorm"

	"shutterseek/internal/config"
)

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	dsn := cfg.Database.DSN()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("connect: %v", err)
	}

	g := gen.NewGenerator(gen.Config{
		OutPath:      "internal/model",
		ModelPkgPath: "internal/model",
		Mode:         gen.WithDefaultQuery | gen.WithQueryInterface,
	})

	g.UseDB(db)

	// Generate models from existing database tables
	g.GenerateModel("photos")
	g.GenerateModel("photo_embeddings")

	g.Execute()

	fmt.Println("✓ Models generated to internal/model/")
}
