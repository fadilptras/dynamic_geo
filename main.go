package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	_ "github.com/lib/pq"
)

// Struktur JSON BMKG
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

func main() {
	// Konfigurasi Konek DB
	connStr := "host=localhost port=5433 user=postgres password=Fputra10 dbname=seatrust_db sslmode=disable"
	
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Gagal membuka koneksi database: ", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		log.Fatal("Database tidak merespons: ", err)
	}
	fmt.Println("[V] Berhasil terhubung ke database seatrust_db!")

	// Tarik Data dari API BMKG
	fmt.Println("[*] Menarik data dari BMKG InaTEWS...")
	url := "https://data.bmkg.go.id/DataMKG/TEWS/autogempa.json"
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal("Error menarik API:", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	var data BMKGResponse
	json.Unmarshal(body, &data)
	gempa := data.Infogempa.Gempa

	// Persiapan Format Koordinat untuk PostGIS
	// BMKG memberikan format: "Latitude,Longitude" (contoh: "-7.12,106.50")
	// PostGIS butuh input: (Longitude, Latitude) -> DIBALIK!
	coords := strings.Split(gempa.Coordinates, ",")
	lat := coords[0]
	lon := coords[1]

	// Proses Insert ke DB
	sqlStatement := `
		INSERT INTO gempabumi (datetime, magnitude, depth, epicenter, location_desc, tsunami_potential)
		VALUES ($1, $2, $3, ST_SetSRID(ST_MakePoint($4, $5), 4326), $6, $7)
		RETURNING id;`

	var insertedID int
	err = db.QueryRow(sqlStatement,
		gempa.DateTime,
		gempa.Magnitude,
		gempa.Kedalaman,
		lon, // Ingat: Longitude duluan untuk ST_MakePoint
		lat,
		gempa.Wilayah,
		gempa.Potensi,
	).Scan(&insertedID)

	if err != nil {
		log.Fatal("Gagal menyimpan ke database: ", err)
	}

	fmt.Printf("[V] Data gempa berhasil disimpan dengan ID: %d\n", insertedID)
	fmt.Println("Wilayah:", gempa.Wilayah)
	fmt.Println("Status Tsunami:", gempa.Potensi)
}