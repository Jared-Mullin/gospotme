package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/cors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/spotify"
)

// Artist Spotify Response Structure
type Artist struct {
	ExternalUrls struct {
		Spotify string `json:"spotify"`
	} `json:"external_urls"`
	Genres []string `json:"genres"`
	Href   string   `json:"href"`
	ID     string   `json:"id"`
	Images []struct {
		Height int    `json:"height"`
		URL    string `json:"url"`
		Width  int    `json:"width"`
	} `json:"images"`
	Name       string `json:"name"`
	Popularity int    `json:"popularity"`
	Type       string `json:"type"`
	URI        string `json:"uri"`
}

type Album struct {
	AlbumType        string   `json:"album_type"`
	Artists          []Artist `json:"artists"`
	AvailableMarkets []string `json:"available_markets"`
	ExternalUrls     struct {
		Spotify string `json:"spotify"`
	} `json:"external_urls"`
	Href   string `json:"href"`
	ID     string `json:"id"`
	Images []struct {
		Height int    `json:"height"`
		URL    string `json:"url"`
		Width  int    `json:"width"`
	} `json:"images"`
	Name                 string `json:"name"`
	ReleaseDate          string `json:"release_date"`
	ReleaseDatePrecision string `json:"release_date_precision"`
	TotalTracks          int    `json:"total_tracks"`
	Type                 string `json:"type"`
	URI                  string `json:"uri"`
}

type Track struct {
	Album            Album    `json:"album"`
	Artists          []Artist `json:"artists"`
	AvailableMarkets []string `json:"available_markets"`
	DiscNumber       int      `json:"disc_number"`
	DurationMs       int      `json:"duration_ms"`
	Explicit         bool     `json:"explicit"`
	ExternalIds      struct {
		Isrc string `json:"isrc"`
	} `json:"external_ids"`
	ExternalUrls struct {
		Spotify string `json:"spotify"`
	} `json:"external_urls"`
	Href        string `json:"href"`
	ID          string `json:"id"`
	IsLocal     bool   `json:"is_local"`
	Name        string `json:"name"`
	Popularity  int    `json:"popularity"`
	PreviewURL  string `json:"preview_url"`
	TrackNumber int    `json:"track_number"`
	Type        string `json:"type"`
	URI         string `json:"uri"`
}
type Items struct {
	Href  string   `json:"href"`
	Items []Artist `json:"items"`
}

type User struct {
	DisplayName  string `json:"display_name"`
	ExternalUrls struct {
		Spotify string `json:"spotify"`
	} `json:"external_urls"`
	SpotifyID string `json:"id"`
	Images    []struct {
		Height interface{} `json:"height"`
		URL    string      `json:"url"`
		Width  interface{} `json:"width"`
	} `json:"images"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

var (
	accessToken  string
	refreshToken string
)

const (
	state = "state"
)

func main() {
	router := chi.NewRouter()
	spotifyConf := setupSpotifyConf()
	mongoClient, mongoContext := createMongoClient()
	defer mongoClient.Disconnect(mongoContext)

	//Middleware Stack
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(60 * time.Second))

	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	router.Get("/spotify/auth", func(w http.ResponseWriter, r *http.Request) {
		url := spotifyConf.AuthCodeURL(state)
		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	})

	router.Get("/spotify/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("state") != state {
			log.Println("Invalid OAuth2 State")
		} else {
			token, err := spotifyConf.Exchange(oauth2.NoContext, r.FormValue("code"))
			if err != nil {
				log.Println(err)
			} else {
				accessToken = token.AccessToken
				refreshToken = token.RefreshToken
				client := http.Client{}
				req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me/", nil)
				if err != nil {
					log.Println("Error in Creating Request")
					log.Println(err)
				} else {
					req.Header.Set("Authorization", "Bearer "+accessToken)
					res, err := client.Do(req)
					if err != nil {
						log.Println("Error in Performing Request")
						log.Println(err)
					} else {
						defer res.Body.Close()
						body, err := ioutil.ReadAll(res.Body)
						if err != nil {
							http.Error(w, err.Error(), http.StatusBadRequest)
							log.Println(err)
						} else {
							var user User
							json.Unmarshal(body, &user)
							user.AccessToken = accessToken
							user.RefreshToken = refreshToken
							userCollection := mongoClient.Database("test").Collection("users")
							ctx, _ := context.WithTimeout(context.Background(), 15*time.Second)
							bUser, _ := bson.Marshal(user)
							exists, err := userCollection.CountDocuments(ctx, bson.M{"spotifyid": user.SpotifyID})
							if err != nil {
								log.Println("Error in Querying users Collection")
								log.Println(err)
							} else if exists == 1 {
								log.Println("User Already Exists")
							} else {
								res, err := userCollection.InsertOne(ctx, bUser)
								if err != nil {
									log.Println("Error in Performing Request")
									log.Println(err)
								} else {
									log.Println(res)
								}
							}
						}
					}
				}
			}
		}
	})

	router.Get("/spotify/artists", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(accessToken)
		client := http.Client{}
		req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me/top/artists", nil)
		if err != nil {
			log.Println("Error in Creating Request")
			log.Println(err)
		} else {
			req.Header.Set("Authorization", "Bearer "+accessToken)
			res, err := client.Do(req)
			if err != nil {
				log.Println("Error in Performing Request")
				log.Println(err)
			} else {
				defer res.Body.Close()
				body, err := ioutil.ReadAll(res.Body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					log.Println(err)
				}
				w.Write(body)
			}
		}
	})

	router.Get("/spotify/tracks", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(accessToken)
		client := http.Client{}
		req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me/top/tracks", nil)
		if err != nil {
			log.Println("Error in Creating Request")
			log.Println(err)
		} else {
			req.Header.Set("Authorization", "Bearer "+accessToken)
			res, err := client.Do(req)
			if err != nil {
				log.Println("Error in Performing Request")
				log.Println(err)
			} else {
				defer res.Body.Close()
				body, err := ioutil.ReadAll(res.Body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					log.Println(err)
				} else {
					w.Write(body)
				}
			}
		}
	})

	router.Get("/spotify/genres", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(accessToken)
		client := http.Client{}
		req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me/top/artists", nil)
		if err != nil {
			log.Println("Error in Creating Request")
			log.Println(err)
		} else {
			req.Header.Set("Authorization", "Bearer "+accessToken)
			res, err := client.Do(req)
			if err != nil {
				log.Println("Error in Performing Request")
				log.Println(err)
			} else {
				defer res.Body.Close()
				body, err := ioutil.ReadAll(res.Body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					log.Println(err)
				} else {
					genres := make(map[string]int)
					var itemWrapper Items
					json.Unmarshal(body, &itemWrapper)
					for _, artist := range itemWrapper.Items {
						for _, genre := range artist.Genres {
							genres[genre] = genres[genre] + 1
						}
					}
					fmt.Println(genres)
					response, err := json.Marshal(genres)
					if err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						log.Println(err)
					} else {
						w.Write(response)
					}
				}
			}
		}
	})

	http.ListenAndServe(":4200", router)
}

func setupSpotifyConf() *oauth2.Config {
	cID := os.Getenv("SPOTIFY_CLIENT_ID")
	cSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	conf := &oauth2.Config{
		ClientID:     cID,
		ClientSecret: cSecret,
		RedirectURL:  "http://localhost:4200/spotify/callback",
		Scopes:       []string{"user-read-private", "user-read-email", "user-top-read"},
		Endpoint:     spotify.Endpoint,
	}
	return conf
}

func createMongoClient() (*mongo.Client, context.Context) {
	uri := "mongodb+srv://" + os.Getenv("DB_USERNAME") + ":" + os.Getenv("DB_PASSWORD") + "@" + os.Getenv("DB_URL") + "/test?retryWrites=true&w=majority"
	client, err := mongo.NewClient(options.Client().ApplyURI(uri))
	if err != nil {
		log.Fatal(err)
	}
	ctx, _ := context.WithTimeout(context.Background(), 60*time.Second)
	err = client.Connect(ctx)
	if err != nil {
		log.Fatal(err)
	}
	return client, ctx
}
