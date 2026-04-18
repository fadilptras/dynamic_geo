package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

var db *sql.DB

func main() {
	fmt.Println("=== Memulai SEA-TRUST Multi-Zone Geofence Engine ===")

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
	log.Println("[+] Database Terhubung! Engine siap memantau seluruh zona.")

	// Engine berjalan tanpa henti setiap 1 menit
	for {
		log.Println("-------------------------------------------------")
		log.Println("[ENGINE] Memulai pemindaian zona...")
		evaluasiMultiZona()
		log.Println("[ENGINE] Pemindaian selesai. Menunggu siklus berikutnya...")
		time.Sleep(1 * time.Minute)
	}
}

func evaluasiMultiZona() {
	// A. TARIK DATA GLOBAL (Maritim, Cuaca, Gempa Terakhir)
	// 1. Data Maritim (Prioritas 0 & 2)
	var maritimTime time.Time
	var ombak, angin, arus, jarakPandang float64
	errMaritim := db.QueryRow(`SELECT dibuat_pada, tinggi_gelombang_max, kecepatan_angin_max, kecepatan_arus, jarak_pandang_km FROM log_parameter_maritim ORDER BY dibuat_pada DESC LIMIT 1`).Scan(&maritimTime, &ombak, &angin, &arus, &jarakPandang)

	// 2. Data Cuaca (Prioritas 3)
	var adaBadai bool
	db.QueryRow(`SELECT EXISTS(SELECT 1 FROM peringatan_dini_cuaca WHERE dibuat_pada >= NOW() - INTERVAL '3 hours')`).Scan(&adaBadai)

	// 3. Data Gempa (Prioritas 1)
	var gempaPotensi string
	var gempaMag float64
	var gempaLon, gempaLat float64
	// Ambil koordinat episentrum gempa untuk dihitung jaraknya per-zona nanti
	errGempa := db.QueryRow(`SELECT tsunami_potential, magnitude, ST_X(epicenter), ST_Y(epicenter) FROM gempabumi ORDER BY datetime DESC LIMIT 1`).Scan(&gempaPotensi, &gempaMag, &gempaLon, &gempaLat)

	// B. LOOPING EVALUASI SETIAP ZONA DI DATABASE
	// Mengambil ID, Nama, dan Titik Tengah (Centroid) dari setiap poligon zona
	rows, err := db.Query(`SELECT id, nama_zona, ST_X(ST_Centroid(polygon)) AS lon, ST_Y(ST_Centroid(polygon)) AS lat FROM zona_geofence`)
	if err != nil {
		log.Println("[-] Gagal mengambil data zona:", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var idZona int
		var namaZona string
		var zonaLon, zonaLat float64
		rows.Scan(&idZona, &namaZona, &zonaLon, &zonaLat)

		var daftarAlasan []string
		warnaFinal := "HIJAU"

		// --- CEK PRIORITAS 0: KADALUARSA DATA ---
		if errMaritim != nil || time.Since(maritimTime).Hours() > 3.0 {
			warnaFinal = "ABU-ABU"
			daftarAlasan = append(daftarAlasan, "Sistem kehilangan sinyal satelit laut (Blind Spot)")
			updateZona(idZona, namaZona, warnaFinal, daftarAlasan)
			continue // Skip ke zona berikutnya karena data tidak valid
		}

		// --- CEK PRIORITAS 1: GEMPA & TSUNAMI ---
		if errGempa == nil {
			// Hitung jarak gempa ke Titik Tengah Zona ini (menggunakan Query ringan)
			var jarakGempaKm float64
			db.QueryRow(`SELECT (ST_DistanceSphere(ST_SetSRID(ST_MakePoint($1, $2), 4326), ST_SetSRID(ST_MakePoint($3, $4), 4326)) / 1000)`, gempaLon, gempaLat, zonaLon, zonaLat).Scan(&jarakGempaKm)

			isTsunami := !strings.Contains(strings.ToLower(gempaPotensi), "tidak berpotensi tsunami")
			if isTsunami && jarakGempaKm <= 1000.0 {
				warnaFinal = "MERAH BERKEDIP"
				daftarAlasan = append(daftarAlasan, fmt.Sprintf("Ancaman Tsunami (Radius %.0fKM)", jarakGempaKm))
			} else if gempaMag >= 5.5 && jarakGempaKm <= 100.0 {
				warnaFinal = "MERAH BERKEDIP"
				daftarAlasan = append(daftarAlasan, fmt.Sprintf("Guncangan Gempa M%.1f Jarak Dekat", gempaMag))
			}
		}

		// --- CEK PRIORITAS 2: FISIK LAUT EKSTREM ---
		if ombak > 2.0 {
			if warnaFinal != "MERAH BERKEDIP" { warnaFinal = "MERAH" }
			daftarAlasan = append(daftarAlasan, fmt.Sprintf("Ombak Tinggi (%.1f m)", ombak))
		}
		if angin > 27.0 {
			if warnaFinal != "MERAH BERKEDIP" { warnaFinal = "MERAH" }
			daftarAlasan = append(daftarAlasan, fmt.Sprintf("Angin Badai (%.1f km/j)", angin))
		}
		if arus > 1.5 {
			if warnaFinal != "MERAH BERKEDIP" { warnaFinal = "MERAH" }
			daftarAlasan = append(daftarAlasan, fmt.Sprintf("Arus Kuat (%.1f m/s)", arus))
		}

		// --- CEK PRIORITAS 3: WASPADA ---
		if adaBadai {
			if warnaFinal == "HIJAU" { warnaFinal = "KUNING" }
			daftarAlasan = append(daftarAlasan, "Peringatan Badai Aktif")
		}
		if ombak >= 1.2 && ombak <= 2.0 {
			if warnaFinal == "HIJAU" { warnaFinal = "KUNING" }
			daftarAlasan = append(daftarAlasan, fmt.Sprintf("Ombak Sedang (%.1f m)", ombak))
		}
		if arus >= 1.0 && arus <= 1.5 {
			if warnaFinal == "HIJAU" { warnaFinal = "KUNING" }
			daftarAlasan = append(daftarAlasan, fmt.Sprintf("Arus Lumayan Kuat (%.1f m/s)", arus))
		}
		if jarakPandang < 2.0 {
			if warnaFinal == "HIJAU" { warnaFinal = "KUNING" }
			daftarAlasan = append(daftarAlasan, fmt.Sprintf("Jarak Pandang Buruk (%.1f km)", jarakPandang))
		}

		// Jika tidak ada masalah
		if len(daftarAlasan) == 0 {
			daftarAlasan = append(daftarAlasan, "Aman Melaut")
		}

		// --- UPDATE KE DATABASE ---
		updateZona(idZona, namaZona, warnaFinal, daftarAlasan)
	}
}

func updateZona(id int, nama string, warna string, alasan []string) {
	alasanFinal := strings.Join(alasan, " + ")
	log.Printf("[ZONA UPDATE] %s | %s | %s\n", nama, warna, alasanFinal)

	_, err := db.Exec(`UPDATE zona_geofence SET status_warna = $1, list_peringatan = $2, terakhir_dievaluasi = CURRENT_TIMESTAMP WHERE id = $3`, warna, alasanFinal, id)
	if err != nil {
		log.Println("[-] Gagal update zona ke DB:", err)
	}
}