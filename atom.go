package main

import (
	"bytes"
	"encoding/xml"
	"log"
	"time"
)

type AtomFeed struct {
	ID      string `xml:"id"`
	Entries []struct {
		ID   string `xml:"id"`
		Link struct {
			Href string `xml:"href,attr"`
		} `xml:"link"`
		Published string `xml:"published"`
		Title     string `xml:"title"`
	} `xml:"entry"`
}

func getAtomPosts(body []byte, feed Feed) ([]NewPost, error) {
	var atomFeed AtomFeed
	if err := xml.NewDecoder(bytes.NewReader(body)).Decode(&atomFeed); err != nil {
		return nil, err
	}

	var posts []NewPost
	for _, entry := range atomFeed.Entries {
		date, err := time.Parse(time.RFC3339, entry.Published)
		if err != nil {
			log.Printf("error happened while parsing published date for atom post from from %s: %v\n", feed.URL, err)
			continue
		}

		post := NewPost{
			Title:  entry.Title,
			URL:    entry.Link.Href,
			Date:   SQLTime{date},
			FeedID: feed.ID,
			UID:    generatePostUID(feed.ID, entry.ID),
		}
		posts = append(posts, post)
	}

	return posts, nil
}
