package main

import (
	"database/sql"
	"html/template"
	"log"
	"net/http"
	"os"

	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"context"
	"time"

	"net"

	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB
var tpl *template.Template
var useHTTPS bool

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

const (
	serverPort  = 8080
	shutdownPort = 8081
	idOffset     = 12345678 // You can adjust this value as needed
)

func init() {
	log.Println("Initializing...")
	var err error
	db, err = sql.Open("sqlite3", "urls.db")
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}
	log.Println("Database opened successfully")

	// Create table if not exists
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS urls (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		original TEXT NOT NULL,
		short TEXT NOT NULL UNIQUE
	)`)
	if err != nil {
		log.Fatal("Failed to create table:", err)
	}
	log.Println("Table created or already exists")

	// Create index on 'original' column
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_original ON urls (original)`)
	if err != nil {
		log.Fatal("Failed to create index:", err)
	}
	log.Println("Index created or already exists")

	tpl = template.Must(template.ParseFiles("index.html"))
	log.Println("Template parsed")

	// Check for an environment variable to determine if HTTPS should be used
	useHTTPS = os.Getenv("USE_HTTPS") == "true"
	log.Println("Initialization complete")
}

func main() {
	log.Println("Starting main function")
	r := mux.NewRouter()
	log.Println("Router created")

	// Root path handler
	r.HandleFunc("/", homeHandler).Methods("GET")

	// App routes
	app := r.PathPrefix("/app").Subrouter()
	app.HandleFunc("/shorten", shortenHandler).Methods("POST")
	app.PathPrefix("/static/").Handler(http.StripPrefix("/app/static/", http.FileServer(http.Dir("."))))

	// Short URL redirect route
	r.HandleFunc("/{shortCode}", redirectHandler).Methods("GET")
	log.Println("Routes registered")

	srv := &http.Server{
		Addr:    ":" + strconv.Itoa(serverPort),
		Handler: r,
	}

	// Channel to signal shutdown
	shutdown := make(chan struct{})

	// Start shutdown listener
	go listenForShutdown(shutdown)

	// Run server in a goroutine
	go func() {
		log.Printf("Server starting on port %d", serverPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe(): %v", err)
		}
	}()

	// Wait for shutdown signal
	<-shutdown
	log.Println("Shutting down server...")

	// Create a deadline to wait for
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exiting")
}

func listenForShutdown(shutdown chan<- struct{}) {
	listener, err := net.Listen("tcp", ":"+strconv.Itoa(shutdownPort))
	if err != nil {
		log.Fatalf("Failed to listen on shutdown port: %v", err)
	}
	defer listener.Close()

	log.Printf("Listening for shutdown signal on port %d", shutdownPort)

	conn, err := listener.Accept()
	if err != nil {
		log.Printf("Error accepting connection: %v", err)
		return
	}
	conn.Close()

	log.Println("Shutdown signal received")
	close(shutdown)
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		ShortURL string
		Error    string
	}{
		ShortURL: r.URL.Query().Get("short"),
		Error:    r.URL.Query().Get("error"),
	}
	tpl.Execute(w, data)
}

func shortenHandler(w http.ResponseWriter, r *http.Request) {
	longURL := r.FormValue("url")

	// Validate URL
	if _, err := url.ParseRequestURI(longURL); err != nil {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	// Check if URL already exists in database
	var shortCode string
	err := db.QueryRow("SELECT short FROM urls WHERE original = ?", longURL).Scan(&shortCode)
	if err == nil {
		// URL already exists, use the existing short code
	} else if err != sql.ErrNoRows {
		// An unexpected error occurred
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	} else {
		// Generate a new short code
		tempCode, err := generateTempCode()
		if err != nil {
			http.Error(w, "Error generating temporary code", http.StatusInternalServerError)
			return
		}

		// Insert new row with temporary code and get the auto-incremented ID
		result, err := db.Exec("INSERT INTO urls (original, short) VALUES (?, ?)", longURL, tempCode)
		if err != nil {
			http.Error(w, "Error saving URL", http.StatusInternalServerError)
			return
		}

		id, err := result.LastInsertId()
		if err != nil {
			http.Error(w, "Error getting insert ID", http.StatusInternalServerError)
			return
		}

		// Generate final short code from ID
		shortCode = generateShortCode(id)

		// Update the row with the final short code
		_, err = db.Exec("UPDATE urls SET short = ? WHERE id = ?", shortCode, id)
		if err != nil {
			http.Error(w, "Error updating short code", http.StatusInternalServerError)
			return
		}
	}

	// Create the full short URL
	scheme := "http"
	if useHTTPS {
		scheme = "https"
	}
	shortURL := fmt.Sprintf("%s://%s/%s", scheme, r.Host, shortCode)

	// Redirect to the home page with the short URL as a query parameter
	http.Redirect(w, r, "/?short="+url.QueryEscape(shortURL), http.StatusSeeOther)
}

func generateTempCode() (string, error) {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b)[:8], nil
}

func generateShortCode(id int64) string {
	// Add the offset to the id
	id += idOffset

	if id == idOffset {
		return string(base62Chars[0])
	}

	var encoded string
	for id > 0 {
		encoded = string(base62Chars[id%62]) + encoded
		id /= 62
	}

	return encoded
}

func redirectHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	shortCode := vars["shortCode"]

	longURL, err := getLongURL(shortCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Short code not found, redirect to home with error message
			http.Redirect(w, r, "/?error="+url.QueryEscape("Short URL not found"), http.StatusSeeOther)
		} else {
			// Database error
			http.Error(w, "Database error", http.StatusInternalServerError)
		}
		return
	}

	// Redirect to the long URL
	http.Redirect(w, r, longURL, http.StatusMovedPermanently)
}

func getLongURL(shortCode string) (string, error) {
	var longURL string
	err := db.QueryRow("SELECT original FROM urls WHERE short = ?", shortCode).Scan(&longURL)
	if err != nil {
		return "", err
	}
	return longURL, nil
}

// TODO: Implement helper functions for URL shortening, database operations, etc.