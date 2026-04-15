package main

import (
	"encoding/json"
	"encoding/xml"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

type PeringatanCuaca struct {
	XMLName xml.Name `xml:"data"`
	Warning struct {
		Tanggal string `xml:"Tanggal"`
		Narasi  string `xml:"Isi"`
	} `xml:"peringatan"`
}

func fetchAndSaveCuaca() error {
	url := "https://data.bmkg.go.id/DataMKG/MEWS/PeringatanDini/peringatandini_jabar.xml"
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var data PeringatanCuaca
	if err := xml.Unmarshal(body, &data); err != nil || data.Warning.Narasi == "" {
		return err
	}

	narasi := data.Warning.Narasi
	var exists bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM peringatan_dini_cuaca WHERE deskripsi_lengkap = $1)", narasi).Scan(&exists)
	if exists {
		return nil
	}

	wilayah := "Jawa Barat (Umum)"
	if strings.Contains(strings.ToLower(narasi), "sukabumi") || strings.Contains(strings.ToLower(narasi), "pelabuhanratu") {
		wilayah = "Sukabumi & Sekitarnya"
	}

	judul := "Peringatan Dini Cuaca " + data.Warning.Tanggal
	sqlStatement := `INSERT INTO peringatan_dini_cuaca (judul, deskripsi_lengkap, wilayah_terdampak) VALUES ($1, $2, $3) RETURNING id;`
	
	var insertedID int
	if err := db.QueryRow(sqlStatement, judul, narasi, wilayah).Scan(&insertedID); err == nil {
		log.Printf("[CUACA] Peringatan baru disimpan (ID: %d)\n", insertedID)
	}
	return err
}

func triggerFetchCuaca(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fetchAndSaveCuaca()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
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