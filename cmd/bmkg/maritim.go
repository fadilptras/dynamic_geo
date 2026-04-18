package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

// Format JSON dari Open-Meteo Marine
type OpenMeteoMarine struct {
	Hourly struct {
		WaveHeight            []float64 `json:"wave_height"`
		OceanCurrentVelocity  []float64 `json:"ocean_current_velocity"`
		OceanCurrentDirection []float64 `json:"ocean_current_direction"`
	} `json:"hourly"`
}

// Format JSON dari Open-Meteo Weather
type OpenMeteoWeather struct {
	Current struct {
		WindSpeed  float64 `json:"wind_speed_10m"`
		Visibility float64 `json:"visibility"` // Dalam meter
	} `json:"current"`
}

func fetchAndSaveMaritim() error {
	// 1. Tarik Data Marine (Gelombang + Arus) untuk Pelabuhanratu
	urlWave := "https://marine-api.open-meteo.com/v1/marine?latitude=-7.03&longitude=106.53&hourly=wave_height,ocean_current_velocity,ocean_current_direction&timezone=Asia%2FJakarta"
	respWave, err := http.Get(urlWave)
	if err != nil || respWave.StatusCode != 200 {
		return fmt.Errorf("gagal hit API Gelombang Open-Meteo")
	}
	defer respWave.Body.Close()
	bodyWave, _ := ioutil.ReadAll(respWave.Body)
	var marineData OpenMeteoMarine
	json.Unmarshal(bodyWave, &marineData)

	// 2. Tarik Data Cuaca (Angin + Visibilitas) untuk Pelabuhanratu
	urlWind := "https://api.open-meteo.com/v1/forecast?latitude=-7.03&longitude=106.53&current=wind_speed_10m,visibility&timezone=Asia%2FJakarta"
	respWind, err := http.Get(urlWind)
	if err != nil || respWind.StatusCode != 200 {
		return fmt.Errorf("gagal hit API Angin Open-Meteo")
	}
	defer respWind.Body.Close()
	bodyWind, _ := ioutil.ReadAll(respWind.Body)
	var weatherData OpenMeteoWeather
	json.Unmarshal(bodyWind, &weatherData)

	// 3. Olah Data
	ombak := 0.0
	kecepatanArus := 0.0
	arahArus := 0.0

	// Mengambil data indeks pertama (jam saat ini) jika datanya tersedia
	if len(marineData.Hourly.WaveHeight) > 0 {
		ombak = marineData.Hourly.WaveHeight[0]
	}
	if len(marineData.Hourly.OceanCurrentVelocity) > 0 {
		kecepatanArus = marineData.Hourly.OceanCurrentVelocity[0] // Satuan km/jam
		// Konversi ke m/s
		kecepatanArus = kecepatanArus * (1000.0 / 3600.0) 
	}
	if len(marineData.Hourly.OceanCurrentDirection) > 0 {
		arahArus = marineData.Hourly.OceanCurrentDirection[0] // Derajat (0-360)
	}

	angin := weatherData.Current.WindSpeed // km/jam
	jarakPandangKm := weatherData.Current.Visibility / 1000.0 

	lokasi := "Perairan Pelabuhanratu (Sukabumi)"
	cuaca := "Terpantau Satelit"

	// 4. Simpan ke Database
	sqlStatement := `
		INSERT INTO log_parameter_maritim (
			nama_wilayah, tinggi_gelombang_max, kecepatan_angin_max, 
			kecepatan_arus, arah_arus, jarak_pandang_km, kondisi_cuaca
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id;`

	var insertedID int
	err = db.QueryRow(sqlStatement, lokasi, ombak, angin, kecepatanArus, arahArus, jarakPandangKm, cuaca).Scan(&insertedID)
	
	if err == nil {
		log.Printf("[MARITIM] SUKSES (ID: %d) | Ombak: %.2fm, Angin: %.2fkm/h, Arus: %.2fm/s, Jarak Pandang: %.1fkm\n", 
			insertedID, ombak, angin, kecepatanArus, jarakPandangKm)
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
	json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Proses Maritim lengkap berhasil ditarik"})
}

func getLatestMaritim(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	sqlStatement := `
		SELECT 
			id, nama_wilayah, tinggi_gelombang_max, kecepatan_angin_max, 
			kecepatan_arus, arah_arus, jarak_pandang_km, kondisi_cuaca 
		FROM log_parameter_maritim ORDER BY id DESC LIMIT 1;`

	var id int
	var lokasi, cuaca string
	var ombak, angin, arus, arah, jarak float64

	err := db.QueryRow(sqlStatement).Scan(&id, &lokasi, &ombak, &angin, &arus, &arah, &jarak, &cuaca)
	if err != nil {
		http.Error(w, "Data tidak ditemukan", http.StatusNotFound)
		return
	}
	
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id": id, "lokasi": lokasi, "ombak_max": ombak, "angin_max": angin,
		"kecepatan_arus": arus, "arah_arus": arah, "jarak_pandang_km": jarak, "cuaca": cuaca,
	})
}