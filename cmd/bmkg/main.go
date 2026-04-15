package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	_ "github.com/lib/pq"
	"github.com/robfig/cron/v3"
)

var db *sql.DB

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "OK",
		"service": "Sea-Trust Ingestion Service",
		"time":    time.Now().Format(time.RFC3339),
	})
}

func main() {
	fmt.Println("=== Memulai Sea-Trust Ingestion Service ===")

	var err error
	connStr := "host=localhost port=5433 user=postgres password=Fputra10 dbname=seatrust_db sslmode=disable"
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Gagal inisialisasi database: ", err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatal("Database tidak merespons: ", err)
	}
	log.Println("[+] Database Terhubung!")

	location, _ := time.LoadLocation("Asia/Jakarta")
	c := cron.New(cron.WithLocation(location))

	c.AddFunc("*/5 * * * *", func() { fetchAndSaveInaTEWS() })
	c.AddFunc("*/15 * * * *", func() { fetchAndSaveCuaca() })
	c.AddFunc("0 * * * *", func() { fetchAndSaveMaritim() })
	c.Start()

	fetchAndSaveInaTEWS()

	http.HandleFunc("/api/health", healthCheckHandler)
	http.HandleFunc("/api/fetch/inatews", triggerFetchInaTEWS)
	http.HandleFunc("/api/earthquakes/latest", getLatestEarthquake)
	http.HandleFunc("/api/fetch/cuaca", triggerFetchCuaca)
	http.HandleFunc("/api/cuaca/latest", getLatestCuaca)
	http.HandleFunc("/api/fetch/maritim", triggerFetchMaritim)
	http.HandleFunc("/api/maritim/latest", getLatestMaritim)

	port := ":8080"
	log.Printf("[+] REST API Server berjalan di http://localhost%s\n", port)

	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal("Server error: ", err)
	}
}