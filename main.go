package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

type NewPost struct {
	Title  string  `json:"title" db:"title"`
	URL    string  `json:"url" db:"url"`
	FeedID int64   `json:"feed_id" db:"feed_url"`
	UID    string  `json:"uid" db:"uid"`
	Date   SQLTime `json:"date" db:"date"`
}

type Post struct {
	ID        int64   `json:"id" db:"id"`
	Title     string  `json:"title" db:"title"`
	URL       string  `json:"url" db:"url"`
	FeedID    int64   `json:"feed_id" db:"feed_id"`
	UID       string  `json:"uid" db:"uid"`
	Date      SQLTime `json:"date" db:"date"`
	CreatedAt SQLTime `json:"created_at" db:"created_at"`
}

type Feed struct {
	ID          int64    `json:"id" db:"id"`
	URL         string   `json:"url" db:"url"`
	LastChecked *SQLTime `json:"last_checked" db:"last_checked"`
	CreatedAt   SQLTime  `json:"created_at" db:"created_at"`
}

type URLFeed struct {
	XMLName xml.Name
}

var ErrRequestTimeout = errors.New("request timed out")

func generatePostUID(feedId int64, postId string) string {
	return fmt.Sprintf("%d,%s", feedId, postId)
}

func getFeeds(db *sqlx.DB) ([]Feed, error) {
	rows, err := db.Queryx("SELECT * FROM feeds")
	if err != nil {
		return nil, err
	}

	var feeds []Feed
	for rows.Next() {
		var feed Feed
		if err := rows.StructScan(&feed); err != nil {
			return nil, err
		}

		feeds = append(feeds, feed)
	}

	return feeds, nil
}

func fetchFeed(feed Feed) ([]NewPost, error) {
	c := http.Client{}
	c.Timeout, _ = time.ParseDuration("10s")
	resp, err := c.Get(feed.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		if resp.StatusCode == 429 {
			return nil, ErrRequestTimeout
		}
		return nil, fmt.Errorf("request failed with status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var urlFeed URLFeed
	if err := xml.NewDecoder(bytes.NewReader(data)).Decode(&urlFeed); err != nil {
		return nil, err
	}

	if urlFeed.XMLName.Local == "feed" {
		// atom feed
		return getAtomPosts(data, feed)
	} else if urlFeed.XMLName.Local == "rss" {
		// rss feed
		return getRSSPosts(data, feed)
	}

	return nil, fmt.Errorf("unsupported feed format: %v", urlFeed)
}

func updateFeedLastChecked(db *sqlx.DB, feed Feed) error {
	formattedDate := time.Now().UTC().Format(time.DateTime)
	if _, err := db.Exec("UPDATE feeds SET last_checked = ? WHERE id = ?", formattedDate, feed.ID); err != nil {
		return err
	}

	return nil
}

func fetchPosts(db *sqlx.DB) {
	for {
		feeds, err := getFeeds(db)
		if err != nil {
			continue
		}

		for _, feed := range feeds {
			if feed.LastChecked != nil {
				timeDiff := time.Now().UTC().Sub(feed.LastChecked.Time)
				if timeDiff < time.Minute*15 {
					continue
				}
			}

			time.Sleep(1 * time.Minute)

			newPosts, err := fetchFeed(feed)

			if err != nil {
				log.Printf("error happened while fetching posts for %s: %v\n", feed.URL, err)
				if errors.Is(err, ErrRequestTimeout) {
					time.Sleep(1 * time.Minute)
					continue
				}

				if err := updateFeedLastChecked(db, feed); err != nil {
					continue
				}
				continue
			}

			if len(newPosts) == 0 { // idk why would this happen, but sure
				continue
			}

			_ = updateFeedLastChecked(db, feed)

			newestPost := newPosts[0]

			result, err := db.Exec("INSERT INTO posts (title, url, feed_id, date, uid) VALUES (?, ?, ?, ?, ?)", newestPost.Title, newestPost.URL, newestPost.FeedID, newestPost.Date.FormatToDB(), newestPost.UID)
			if err != nil {
				log.Printf("error happened while saving post to db: %v", err)
				continue
			}

			id, _ := result.LastInsertId()
			fmt.Printf("saved post: %d\n", id)
		}
	}
}

func getPosts(db *sqlx.DB, limit, offset int) ([]Post, error) {
	posts := []Post{}

	rows, err := db.Queryx("SELECT * FROM posts LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var post Post
		if err := rows.StructScan(&post); err != nil {
			return nil, err
		}
		posts = append(posts, post)
	}

	return posts, nil
}

func sendJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

// getPaginationParams returns limit, offset and error
func getPaginationParams(r *http.Request, defaultLimit int) (int, int, error) {
	q := r.URL.Query()
	limit := q.Get("limit")
	page := q.Get("page")
	if limit == "" {
		limit = strconv.Itoa(defaultLimit)
	}
	if page == "" {
		page = "1"
	}

	l, err := strconv.Atoi(limit)
	if err != nil {
		return -1, -1, err
	}
	p, err := strconv.Atoi(page)
	if err != nil {
		return -1, -1, err
	}

	offset := (p - 1) * l

	return l, offset, nil
}

func main() {
	db := sqlx.MustOpen("sqlite3", "./db.db")

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	go fetchPosts(db)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /feeds", func(w http.ResponseWriter, r *http.Request) {
		feeds, err := getFeeds(db)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		sendJSON(w, feeds)
	})

	mux.HandleFunc("GET /posts", func(w http.ResponseWriter, r *http.Request) {
		limit, offset, err := getPaginationParams(r, 10)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		posts, err := getPosts(db, limit, offset)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		sendJSON(w, posts)
	})

	mux.HandleFunc("POST /feeds", func(w http.ResponseWriter, r *http.Request) {
		type body struct {
			URL string `json:"url"`
		}

		var b body
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		result, err := db.Exec("INSERT INTO feeds (url) VALUES (?)", b.URL)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		id, _ := result.LastInsertId()
		sendJSON(w, struct {
			ID int64 `json:"id"`
		}{id})
	})

	if err := http.ListenAndServe(":5000", mux); err != nil {
		log.Fatal(err)
	}
}
