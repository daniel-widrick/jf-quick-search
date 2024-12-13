package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	//"net/http/httputil"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
)

type Item struct {
	Name string `json:"Name"`
	Id string `json:"Id"`
	PremiereDate string `json:"PremiereDate"`
	Artists []string `json:"Artists"`
	Album	string `json:"Album"`
}

type Data struct {
	Items []Item `json:"items"`
}

func ProxyHandler(w http.ResponseWriter, r *http.Request){
	segments := strings.Split(r.URL.Path, "/")
	if len(segments) < 4 {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	id := segments[2]
	file := segments[3]

	jellyfinAddr := os.Getenv("jellyfinAddr")
	audioURL := fmt.Sprintf("http://%s/Audio/%s/%s", jellyfinAddr, id, file)
	log.Printf("Proxying: %s", audioURL)
	parsedURL, err := r.URL.Parse(audioURL)
	if err != nil {
		http.Error(w, "Failed to parse URL", http.StatusInternalServerError)
		return
	}
	log.Printf("Parsed URL: %v", parsedURL)

	
	resp, err := http.Get(audioURL)
	if err != nil {
		msg := fmt.Sprintf("Error loading stream from jellyfin: %v", err)
		log.Printf(msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		msg := fmt.Sprintf("Error reading response body: %v", err)
		log.Printf(msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	w.Write(body)
	return


	//proxy := httputil.NewSingleHostReverseProxy(parsedURL)
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "no-cache")

	//r.Host = jellyfinAddr
	//proxy.ServeHTTP(w, r)
}

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Unable to load .env file: %v", err)
	}
	apiKey := os.Getenv("apiKey")
	jellyfinAddr := os.Getenv("jellyfinAddr")

	//Fetch all audio items from the jellyfin api
	fetchURL := fmt.Sprintf("http://%s/Items/?apiKey=%s&recursive=true&includeItemTypes=Audio", jellyfinAddr, apiKey)
	resp, err := http.Get(fetchURL)
	if err != nil || resp == nil {
		log.Fatalf("Failed to fetch data from jellyfin: %v", err)
	}
	defer resp.Body.Close()

	//Read the response body
	r, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response body from jellyfin: %v", err)
	}

	//Load json into usable datatype
	var data Data
	err = json.Unmarshal(r, &data)
	if err != nil {
		log.Fatalf("Unable to parse json data: %v", err)
	}
	//log.Printf("Got data\n%v",data)

	//Open or create the database
	db, err := sql.Open("sqlite3", "search.db")
	if err != nil {
		log.Fatalf("Failed to open the database: %v", err )
	}
	defer db.Close()

	//Create the songs table if it doesn't exist
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS songs (
		Id TEXT UNIQUE,
		Name TEXT NOT NULL,
		PremiereDate INTEGER,
		Album TEXT
	)`)
	if err != nil {
		log.Fatalf("Failed to create songs table: %v", err)
	}

	//Create the artists table if it doesn't exit
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS artists (
		Name TEXT UNIQUE
	)`)
	if err != nil {
		log.Fatalf("Failed to create the artists table %v", err)
	}

	//Create the song_artists table
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS song_artists (
		Artist TEXT NOT NULL,
		SongId TEXT NOT NULL,
		UNIQUE(SongId, Artist)
	)`)
	if err != nil {
		log.Fatalf("Failed to create song/artist lookup table: %v", err)
	}

	//Create EVERY INDEX
	//--songs table
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_songs_Id on songs(Id)`)
	if err != nil {
		log.Fatalf("Unable to create index: %v", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_songs_Name on songs(Name)`)
	if err != nil {
		log.Fatalf("Unable to create index: %v", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_song_Album on songs(Album)`)
	if err != nil {
		log.Fatalf("Unable to create index: %v", err)
	}
	//--artists table
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_artists_name on artists(Name)`)
	if err != nil {
		log.Fatalf("Unable to create index: %v", err)
	}
	//--song artist lookup
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_song_artists_artist on song_artists(Artist)`)
	if err != nil {
		log.Fatalf("Unable to create index: %v", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_song_artists_SongId on song_artists(SongId)`)
	if err != nil {
		log.Fatalf("Unable to create index: %v", err)
	}

	//Populate DB
	songInsert := `INSERT INTO songs (Id, Name, PremiereDate, Album)
			VALUES(?, ?, ?, ?)
			ON CONFLICT(Id) DO UPDATE SET
				Name = excluded.Name,
				PremiereDate = excluded.PremiereDate,
				Album = excluded.Album`
	songInsertStmt, err := db.Prepare(songInsert)
	if err != nil {
		log.Fatalf("Unable to prepare song insert statement: %v", err)
	}
	defer songInsertStmt.Close()

	ArtistInsert := `INSERT INTO artists (Name) VALUES(?)
		ON CONFLICT(Name) DO NOTHING`
	ArtistInsertStmt, err := db.Prepare(ArtistInsert)
	if err != nil {
		log.Fatalf("Unable to prepare artists insert statement: %v", err)
	}
	defer ArtistInsertStmt.Close()

	SongArtistInsert := `INSERT INTO song_artists (SongId, Artist) VALUES(?, ?)
		ON CONFLICT(SongId, Artist) DO NOTHING`
	SongArtistInsertStmt, err := db.Prepare(SongArtistInsert)
	if err != nil {
		log.Fatalf("Unable to prepare song_artist insert statement: %v", err)
	}
	defer SongArtistInsertStmt.Close()

	for _, dataItem := range data.Items {
		parsedTime, err := time.Parse("2006-01-02T15:04:05.0000000Z", dataItem.PremiereDate)
		if err != nil {
			log.Printf("Error parsing date for song: %s :: %s. %v", dataItem.Name, dataItem.PremiereDate, err)
			parsedTime = time.Unix(0, 0) // oh well?
		}

		//Insert Song
		_, err = songInsertStmt.Exec(dataItem.Id, dataItem.Name, parsedTime, dataItem.Album)
		if err != nil {
			log.Fatalf("Failed to execute prepared song insert statement: %v", err)
		}

		//Insert Artist
		for _,artist := range dataItem.Artists {
			_, err = ArtistInsertStmt.Exec(artist)
			if err != nil {
				log.Fatalf("Unable to insert artist: %s. %v", artist, err)
			}
			_, err = SongArtistInsertStmt.Exec(dataItem.Id, artist)
			if err != nil {
				log.Fatalf("Unable to insert song_artist: %s :: %s. %v", dataItem.Name, artist, err)
			}
		}
	}

	searchQuery := `SELECT songs.Id, songs.Name, Songs.Album, GROUP_CONCAT(song_artists.Artist, ', ') AS Artists,
		LENGTH(songs.Name) - LENGTH(REPLACE(LOWER(songs.Name), LOWER(?), '')) as name_length,
		LENGTH(GROUP_CONCAT(song_artists.Artist, ', ')) -
			LENGTH(REPLACE(GROUP_CONCAT(LOWER(song_artists.Artist), ', '), LOWER(?), '')) as artist_length,
		LENGTH(songs.Album) - LENGTH(REPLACE(LOWER(songs.Album), LOWER(?), '')) as album_length
		FROM songs LEFT JOIN song_artists ON
			songs.Id = song_artists.SongId
		WHERE
			songs.Name LIKE ? OR
			songs.Album LIKE ? OR
			song_artists.Artist LIKE ?
		GROUP BY songs.Id
		ORDER BY name_length DESC, artist_length DESC, album_length DESC`
	searchQueryStmt,err := db.Prepare(searchQuery)
	if err != nil {
		log.Fatalf("Unable to prepare search query: %v", err)
	}

	//Create Webserver
	mux := http.NewServeMux()
	mux.HandleFunc("/search/", func(w http.ResponseWriter, r *http.Request) {
		//q := "%" + r.PathValue("q") + "%"
		//q := r.PathValue("q")
		q := r.URL.Query().Get("q")
		qw := "%" + q + "%"
		rows, err := searchQueryStmt.Query(q, q, q, qw, qw, qw)
		if err != nil {
			msg := fmt.Sprintf("Error running search query: %s, %v", q, err)
			log.Println(msg)
			http.Error(w, msg, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var songs []Item
		for rows.Next() {
			var artists string
			var song Item
			var r1, r2, r3 int
			if err := rows.Scan(&song.Id, &song.Name, &song.Album, &artists, &r1, &r2, &r3); err != nil {
				msg := fmt.Sprintf("Unable to fetch song: %v", err)
				log.Println(msg)
				continue
			}
			song.Artists = strings.Split(artists, ", ")
			songs = append(songs, song)
		}
		jsonData, err := json.Marshal(songs)
		if err != nil {
			msg := fmt.Sprintf("unable to marshall json: %v", err)
			log.Println(msg)
			http.Error(w, msg, http.StatusInternalServerError)
			return
		}
		//log.Printf(string(jsonData))
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonData)
	})

	//serve index
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request){
		http.ServeFile(w, r, "index.html")
	})

	mux.HandleFunc("/Audio/",ProxyHandler)

	addr := ":7777"
	log.Printf("Starting server on %s", addr)
	http.ListenAndServe(addr, mux)
}
