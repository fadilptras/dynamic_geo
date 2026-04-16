package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

// Struktur XML untuk membaca format RSS CAP dari BMKG
type RSS struct {
	XMLName xml.Name `xml:"rss"`
	Channel Channel  `xml:"channel"`
}

type Channel struct {
	Items []Item `xml:"item"`
}

type Item struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

func fetchAndSaveCuaca() error {
	// Endpoint API Peringatan Dini Cuaca BMKG TERBARU (Format CAP/RSS)
	url := "https://www.bmkg.go.id/alerts/nowcast/id"
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("gagal membuat request: %v", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0 Safari/537.36")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("gagal hit API Cuaca BMKG: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server BMKG menolak (Status: %d). Endpoint bermasalah.", resp.StatusCode)
	}

	body, _ := ioutil.ReadAll(resp.Body)
	var rss RSS
	if err := xml.Unmarshal(body, &rss); err != nil {
		return fmt.Errorf("gagal parsing XML RSS BMKG: %v", err)
	}

	ditemukan := false

	// Cek semua peringatan badai aktif di seluruh Indonesia
	for _, item := range rss.Channel.Items {
		descLower := strings.ToLower(item.Description)
		
		// Filter: Hanya ambil jika menyebut Jawa Barat atau Sukabumi
		if strings.Contains(descLower, "jawa barat") || strings.Contains(descLower, "sukabumi") {
			ditemukan = true
			
			// Ambil 50 karakter pertama untuk log agar terminal rapi
			limit := 50
			if len(item.Description) < 50 { limit = len(item.Description) }
			log.Printf("[CUACA] Menemukan peringatan Jabar: %s...\n", item.Description[:limit])

			// Cek Duplikasi di Database
			var exists bool
			db.QueryRow("SELECT EXISTS(SELECT 1 FROM peringatan_dini_cuaca WHERE deskripsi_lengkap = $1)", item.Description).Scan(&exists)
			if exists {
				log.Println("[CUACA] Peringatan ini sudah ada di DB. Skip insert.")
				continue
			}

			// Penentuan Wilayah Spesifik
			wilayah := "Jawa Barat (Umum)"
			if strings.Contains(descLower, "sukabumi") || strings.Contains(descLower, "pelabuhanratu") {
				wilayah = "Sukabumi & Sekitarnya"
			}

			sqlStatement := `INSERT INTO peringatan_dini_cuaca (judul, deskripsi_lengkap, wilayah_terdampak) VALUES ($1, $2, $3) RETURNING id;`
			
			var insertedID int
			if err := db.QueryRow(sqlStatement, item.Title, item.Description, wilayah).Scan(&insertedID); err == nil {
				log.Printf("[CUACA] SUKSES! Peringatan badai disimpan (ID: %d)\n", insertedID)
			} else {
				log.Printf("[CUACA-ERROR] Gagal simpan ke DB: %v\n", err)
			}
		}
	}

	if !ditemukan {
		log.Println("[CUACA] Saat ini langit Jawa Barat AMAN. Tidak ada peringatan dini aktif dari BMKG.")
	}

	return nil
}

func triggerFetchCuaca(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	err := fetchAndSaveCuaca()
	
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Proses Cuaca BMKG (CAP/RSS) selesai"})
}

func getLatestCuaca(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var id int
	var judul, deskripsi, wilayah string

	err := db.QueryRow(`SELECT id, judul, deskripsi_lengkap, wilayah_terdampak FROM peringatan_dini_cuaca ORDER BY dibuat_pada DESC LIMIT 1`).Scan(&id, &judul, &deskripsi, &wilayah)
	if err != nil {
		http.Error(w, "Data tidak ditemukan", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "judul": judul, "deskripsi": deskripsi, "wilayah": wilayah})
}