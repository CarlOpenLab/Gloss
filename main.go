package main

import (
	"embed"
	"log"
	"net/http"
	"os"

	"diff-explainer/internal/server"
)

//go:embed all:web/dist
var webDist embed.FS

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	dist, err := webDistSub()
	if err != nil {
		log.Fatal(err)
	}
	srv := server.New(dist)
	addr := ":" + port
	log.Printf("✨ diff-explainer 已启动: http://localhost%s", addr)
	log.Printf("   AI 解释需要 ANTHROPIC_API_KEY（或在前端设置里填写）")
	log.Fatal(http.ListenAndServe(addr, srv.Handler()))
}
