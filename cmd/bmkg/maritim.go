package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

// Format JSON dari Open-Meteo Marine (Gelombang)
type OpenMeteoMarine struct {
	Hourly struct {
		WaveHeight []float64 `json:"wave_height"`
	} `json:"hourly"`
}

// Format JSON dari Open-Meteo Weather (Angin)
type OpenMeteoWeather struct {
	Current struct {
		WindSpeed float64 `json:"wind_speed_10m"`
	} `json:"current"`
}

func fetchAndSaveMaritim() error {
	// 1. Tarik Data Gelombang Laut untuk Pelabuhanratu (Sukabumi)
	urlWave := "https://marine-api.open-meteo.com/v1/marine?latitude=-7.03&longitude=106.53&hourly=wave_height&timezone=Asia%2FJakarta"
	respWave, err := http.Get(urlWave)
	if err != nil || respWave.StatusCode != 200 {
		return fmt.Errorf("gagal hit API Gelombang Open-Meteo")
	}
	defer respWave.Body.Close()
	bodyWave, _ := ioutil.ReadAll(respWave.Body)
	var marineData OpenMeteoMarine
	json.Unmarshal(bodyWave, &marineData)

	// 2. Tarik Data Angin Darat/Pantai untuk Pelabuhanratu
	urlWind := "https://api.open-meteo.com/v1/forecast?latitude=-7.03&longitude=106.53&current=wind_speed_10m&timezone=Asia%2FJakarta"
	
	// [PERBAIKAN TYPO] Menggunakan := karena respWind adalah variabel baru
	respWind, err := http.Get(urlWind) 
	
	if err != nil || respWind.StatusCode != 200 {
		return fmt.Errorf("gagal hit API Angin Open-Meteo")
	}
	defer respWind.Body.Close()
	bodyWind, _ := ioutil.ReadAll(respWind.Body)
	var weatherData OpenMeteoWeather
	json.Unmarshal(bodyWind, &weatherData)

	// 3. Olah dan Simpan ke Database
	ombak := 0.0
	if len(marineData.Hourly.WaveHeight) > 0 {
		ombak = marineData.Hourly.WaveHeight[0] // Ambil prakiraan ombak jam ini
	}
	angin := weatherData.Current.WindSpeed // Kecepatan angin saat ini (km/jam)
	lokasi := "Perairan Pelabuhanratu (Sukabumi)"
	cuaca := "Terpantau Satelit" 

	sqlStatement := `
		INSERT INTO log_parameter_maritim (nama_wilayah, tinggi_gelombang_max, kecepatan_angin_max, kondisi_cuaca)
		VALUES ($1, $2, $3, $4) RETURNING id;`

	var insertedID int
	err = db.QueryRow(sqlStatement, lokasi, ombak, angin, cuaca).Scan(&insertedID)
	if err == nil {
		log.Printf("[MARITIM] SUKSES! Log disimpan (ID: %d) - Lokasi: %s | Ombak: %.2fm, Angin: %.2fkm/h\n", insertedID, lokasi, ombak, angin)
	} else {
		return fmt.Errorf("gagal insert database: %v", err)
	}

	return nil
}

func triggerFetchMaritim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	err := fetchAndSaveMaritim()
	
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Proses Maritim berhasil ditarik dari satelit Open-Meteo"})
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