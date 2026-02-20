package main

import (
	"embed"
	"log"

	"luncher/internal/config"
	"luncher/internal/launcher"
)

var buildMode = "dev"
var appVersion = "dev"
var gitCommit = "unknown"

//go:embed templates/** static/**
var embedded embed.FS

func main() {
	log.Printf("Kimmio Launcher %s (%s)", appVersion, gitCommit)
	cfg := config.Load(buildMode)
	if err := launcher.Run(embedded, cfg); err != nil {
		log.Fatal(err)
	}
}
