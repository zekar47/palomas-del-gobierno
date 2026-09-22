package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	addr := flag.String("addr", ":8080", "dirección de escucha")
	dbPath := flag.String("db", "palomas.db", "ruta a la base de datos SQLite")
	uploadDir := flag.String("uploads", "uploads", "directorio de archivos subidos (imágenes/videos)")
	adminPass := flag.String("admin-pass", os.Getenv("PALOMAS_ADMIN_PASSWORD"), "contraseña del admin inicial (o env PALOMAS_ADMIN_PASSWORD)")
	flag.Parse()

	if err := os.MkdirAll(*uploadDir, 0o750); err != nil {
		log.Fatalf("no pude crear el directorio de uploads: %v", err)
	}
	uploadsDir = *uploadDir

	var err error
	dsn := *dbPath + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		log.Fatalf("no pude abrir la base de datos: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(8)

	if err := migrate(); err != nil {
		log.Fatalf("migración fallida: %v", err)
	}
	if err := seedAdmin(*adminPass); err != nil {
		log.Fatalf("no pude crear el admin: %v", err)
	}
	if err := loadTemplates(); err != nil {
		log.Fatalf("no pude cargar las plantillas: %v", err)
	}

	mux := http.NewServeMux()
	routes(mux)

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Println(`
  ╔══════════════════════════════════════════════════════════╗
  ║  ██████╗  █████╗ ██╗      ██████╗ ███╗   ███╗ █████╗ ███████╗
  ║  ██╔══██╗██╔══██╗██║     ██╔═══██╗████╗ ████║██╔══██╗██╔════╝
  ║  ██████╔╝███████║██║     ██║   ██║██╔████╔██║███████║███████╗
  ║  ██╔═══╝ ██╔══██║██║     ██║   ██║██║╚██╔╝██║██╔══██║╚════██║
  ║  ██║     ██║  ██║███████╗╚██████╔╝██║ ╚═╝ ██║██║  ██║███████║
  ║  ╚═╝     ╚═╝  ╚═╝╚══════╝ ╚═════╝ ╚═╝     ╚═╝╚═╝  ╚═╝╚══════╝`)
	fmt.Printf("  sistema en línea: http://localhost%s\n", *addr)
	fmt.Printf("  base de datos:    %s\n", *dbPath)
	fmt.Printf("  uploads:          %s\n", *uploadDir)
	if err := httpSrv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
