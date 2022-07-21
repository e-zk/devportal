package main

import (
	"database/sql"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"

	"github.com/accrescent/devportal/auth"
	"github.com/accrescent/devportal/middleware"
	"github.com/accrescent/devportal/page"
	"github.com/accrescent/devportal/routes"
)

func main() {
	r := gin.New()
	err := r.SetTrustedProxies(nil)
	if err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("sqlite3", "devportal.db?_fk=yes")
	if err != nil {
		log.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		gh_id TEXT NOT NULL,
		access_token TEXT NOT NULL,
		expiry_time INT NOT NULL
	) STRICT`); err != nil {
		log.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS users (
		gh_id TEXT PRIMARY KEY,
		email TEXT NOT NULL
	) STRICT`); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS usable_email_cache (
		session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		email TEXT NOT NULL,
		PRIMARY KEY (session_id, email)
	) STRICT`); err != nil {
		log.Fatal(err)
	}

	oauth2Conf := &oauth2.Config{
		ClientID:     os.Getenv("GH_CLIENT_ID"),
		ClientSecret: os.Getenv("GH_CLIENT_SECRET"),
		Endpoint:     endpoints.GitHub,
		RedirectURL:  "https://localhost:8080/auth/github/callback",
		Scopes:       []string{"user:email"},
	}

	r.LoadHTMLGlob("page/templates/*.html")

	r.Use(middleware.DB(db))
	r.Use(middleware.OAuth2Config(oauth2Conf))

	r.GET("/auth/github", auth.GitHub)
	r.GET("/auth/github/callback", auth.GitHubCallback)

	auth := r.Group("/", middleware.AuthRequired())
	auth.GET("/register", page.Register)
	auth.GET("/portal", page.Portal)
	auth.POST("/register", routes.Register)
	auth.POST("/logout", routes.Logout)

	err = r.Run()
	if err != nil {
		log.Fatal(err)
	}
}
