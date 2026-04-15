package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

type MaritimData struct {
	AreaID       string  `json:"area_id"`
	Lokasi       string  `json:"lokasi"`
	Cuaca        string  `json:"cuaca"`
	AnginMax     float64 `json:"angin_max"`
	GelombangMax float64 `json:"gelombang_max"`
}

func fetchAndSaveMaritim() error {
	url := "https://peta-maritim.bmkg.go.id/public_api/pelayanan/JABAR"
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var dataArray []MaritimData
	if err := json.Unmarshal(body, &dataArray); err != nil {
		return err
	}

	for _, data := range dataArray {
		if strings.Contains(strings.ToLower(data.Lokasi), "sukabumi") {
			sqlStatement := `
				INSERT INTO log_parameter_maritim (nama_wilayah, tinggi_gelombang_max, kecepatan_angin_max, kondisi_cuaca)
				VALUES ($1, $2, $3, $4) RETURNING id;`
			
			var insertedID int
			if err := db.QueryRow(sqlStatement, data.Lokasi, data.GelombangMax, data.AnginMax, data.Cuaca).Scan(&insertedID); err == nil {
				log.Printf("[MARITIM] Log maritim disimpan (ID: %d) - %s\n", insertedID, data.Lokasi)
			}
			break 
		}
	}
	return nil
}

func triggerFetchMaritim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fetchAndSaveMaritim()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func getLatestMaritim(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var id int
	var lokasi, cuaca string
	var ombak, angin float64

	err := db.QueryRow(`SELECT id, nama_wilayah, tinggi_gelombang_max, kecepatan_angin_max, kondisi_cuaca FROM log_parameter_maritim ORDER BY dibuat_pada DESC LIMIT 1`).Scan(&id, &lokasi, &ombak, &angin, &cuaca)
	if err != nil {
		http.Error(w, "Data tidak ditemukan", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id": id, "lokasi": lokasi, "ombak_max": ombak, "angin_max": angin, "cuaca": cuaca,
	})
}