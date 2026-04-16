package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
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

func fetchAndSaveInaTEWS() error {
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

	coords := strings.Split(gempa.Coordinates, ",")
	
	var exists bool
	sqlCheck := `SELECT EXISTS(SELECT 1 FROM gempabumi WHERE datetime = $1 AND location_desc = $2)`
	err = db.QueryRow(sqlCheck, gempa.DateTime, gempa.Wilayah).Scan(&exists)
	if err != nil {
		return fmt.Errorf("gagal cek DB: %v", err)
	}
	
	if exists {
		log.Println("[INATEWS] Data gempa sudah ada di DB. Skip insert.")
		return nil
	}

	sqlStatement := `
		INSERT INTO gempabumi (datetime, magnitude, depth, epicenter, location_desc, tsunami_potential)
		VALUES ($1, $2, $3, ST_SetSRID(ST_MakePoint($4, $5), 4326), $6, $7) RETURNING id;`

	var insertedID int
	err = db.QueryRow(sqlStatement, gempa.DateTime, gempa.Magnitude, gempa.Kedalaman, coords[1], coords[0], gempa.Wilayah, gempa.Potensi).Scan(&insertedID)
	if err == nil {
		log.Printf("[INATEWS] SUKSES! Gempa baru disimpan (ID: %d) - %s\n", insertedID, gempa.Wilayah)
	}
	return err
}

func triggerFetchInaTEWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	err := fetchAndSaveInaTEWS() // [UPDATE]: Tangkap Error
	
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Proses Tsunami selesai"})
}

func getLatestEarthquake(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	sqlStatement := `
		SELECT id, datetime, magnitude, depth, ST_AsText(epicenter), location_desc, tsunami_potential 
		FROM gempabumi ORDER BY datetime DESC LIMIT 1;`

	var id int
	var datetime time.Time
	var mag float64
	var depth, epicenterText, loc, pot string

	if err := db.QueryRow(sqlStatement).Scan(&id, &datetime, &mag, &depth, &epicenterText, &loc, &pot); err != nil {
		http.Error(w, "Data tidak ditemukan", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"id": id, "datetime": datetime, "magnitude": mag,
		"epicenter_wkt": epicenterText, "location": loc, "tsunami_potential": pot,
	})
}