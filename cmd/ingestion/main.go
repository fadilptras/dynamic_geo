package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/robfig/cron/v3"
)


type BMKGResponse struct {
	Infogempa struct {
		Gempa GempaData `json:"gempa"`
	} `json:"Infogempa"`
}

type GempaData struct {
	Tanggal     string `json:"Tanggal"`
	Jam         string `json:"Jam"`
	DateTime    string `json:"DateTime"`
	Coordinates string `json:"Coordinates"`
	Magnitude   string `json:"Magnitude"`
	Kedalaman   string `json:"Kedalaman"`
	Wilayah     string `json:"Wilayah"`
	Potensi     string `json:"Potensi"`
}

// Variabel Global Database
var db *sql.DB

// Narik data dari BMKG dan menyimpan ke DB
func fetchAndSaveInaTEWS() error {
	log.Println("[WORKER] Menarik data Gempa InaTEWS dari BMKG...")
	url := "https://data.bmkg.go.id/DataMKG/TEWS/autogempa.json"
	
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("gagal hit API BMKG: %v", err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var data BMKGResponse
	json.Unmarshal(body, &data)
	gempa := data.Infogempa.Gempa

	// Parsing Koordinat untuk PostGIS
	coords := strings.Split(gempa.Coordinates, ",")
	lat := coords[0]
	lon := coords[1]

	// (mencegah duplikat berdasarkan DateTime & lokasi)
	var exists bool
	
	// tambah parameter kedua ($2) yaitu gempa.Wilayah
	sqlCheck := `
		SELECT EXISTS(
			SELECT 1 FROM gempabumi 
			WHERE datetime = $1 AND location_desc = $2
		)`
		
	err = db.QueryRow(sqlCheck, gempa.DateTime, gempa.Wilayah).Scan(&exists)
	if err != nil {
		return fmt.Errorf("gagal cek duplikasi: %v", err)
	}

	if exists {
		log.Println("[WORKER] Data gempa ini sudah ada di database. Skip insert.")
		return nil
	}

	// Insert ke PostgreSQL + PostGIS
	sqlStatement := `
		INSERT INTO gempabumi (datetime, magnitude, depth, epicenter, location_desc, tsunami_potential)
		VALUES ($1, $2, $3, ST_SetSRID(ST_MakePoint($4, $5), 4326), $6, $7) RETURNING id;`

	var insertedID int
	err = db.QueryRow(sqlStatement, gempa.DateTime, gempa.Magnitude, gempa.Kedalaman, lon, lat, gempa.Wilayah, gempa.Potensi).Scan(&insertedID)
	if err != nil {
		return fmt.Errorf("gagal insert ke database: %v", err)
	}

	log.Printf("[WORKER] SUKSES! Gempa baru disimpan (ID: %d) - %s\n", insertedID, gempa.Wilayah)
	return nil
}

// REST API HANDLERS
// Handler 1: Cek status Service
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "OK",
		"service": "Sea-Trust Ingestion Service",
		"time": time.Now().Format(time.RFC3339),
	})
}

// Handler 2: Trigger manual untuk menarik data (Berguna untuk testing)
func triggerFetchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := fetchAndSaveInaTEWS()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Data BMKG berhasil ditarik dan diperbarui"})
}

// Handler 3: Endpoint untuk Geofence Engine mengambil data gempa terakhir
func getLatestEarthquakeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Query ambil 1 gempa terbaru, konversi PostGIS Point kembali ke Teks (Lat, Lon)
	sqlStatement := `
		SELECT id, datetime, magnitude, depth, ST_AsText(epicenter), location_desc, tsunami_potential 
		FROM gempabumi 
		ORDER BY datetime DESC LIMIT 1;`

	var id int
	var datetime time.Time
	var mag float64
	var depth, epicenterText, loc, pot string

	err := db.QueryRow(sqlStatement).Scan(&id, &datetime, &mag, &depth, &epicenterText, &loc, &pot)
	if err != nil {
		http.Error(w, "Data tidak ditemukan", http.StatusNotFound)
		return
	}

	response := map[string]interface{}{
		"id": id,
		"datetime": datetime,
		"magnitude": mag,
		"epicenter_wkt": epicenterText, // Format: POINT(lon lat)
		"location": loc,
		"tsunami_potential": pot,
	}
	json.NewEncoder(w).Encode(response)
}


func main() {
	fmt.Println("=== Memulai Sea-Trust Ingestion Service ===")

	// Koneksi DB
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

	// Setup Cron Job (Background Task)
	location, _ := time.LoadLocation("Asia/Jakarta")
	c := cron.New(cron.WithLocation(location))
	
	// Tarik data BMKG setiap 5 menit
	c.AddFunc("*/5 * * * *", func() {
		fetchAndSaveInaTEWS()
	})
	c.Start()
	log.Println("[+] Cron Job aktif. Menarik data tiap 5 menit.")

	// Jalankan sekali di awal
	fetchAndSaveInaTEWS()

	// Setup REST API (Foreground Task)
	http.HandleFunc("/api/health", healthCheckHandler)
	http.HandleFunc("/api/fetch/inatews", triggerFetchHandler)
	http.HandleFunc("/api/earthquakes/latest", getLatestEarthquakeHandler)

	port := ":8080"
	log.Printf("[+] REST API Server berjalan di http://localhost%s\n", port)
	
	// Menjalankan server (akan memblokir thread agar program tidak exit)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal("Server error: ", err)
	}
}