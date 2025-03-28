package main

import (
	"bytes"
	"encoding/xml"
	"log"
	"time"
)

type RSSFeed struct {
	Channel struct {
		Items []struct {
			Title   string `xml:"title"`
			Link    string `xml:"link"`
			PubDate string `xml:"pubDate"`
			GUID    string `xml:"guid"`
		} `xml:"item"`
	} `xml:"channel"`
}

func getRSSPosts(body []byte, feed Feed) ([]NewPost, error) {
	var posts []NewPost

	var rssFeed RSSFeed
	if err := xml.NewDecoder(bytes.NewReader(body)).Decode(&rssFeed); err != nil {
		return nil, err
	}

	for _, item := range rssFeed.Channel.Items {
		date, err := time.Parse(time.RFC1123, item.PubDate)
		if err != nil {
			log.Printf("error happened while parsing published date for rss post from from %s: %v\n", feed.URL, err)
			continue
		}

		post := NewPost{
			Title:  item.Title,
			URL:    item.Link,
			FeedID: feed.ID,
			UID:    generatePostUID(feed.ID, item.GUID),
			Date:   SQLTime{date},
		}
		posts = append(posts, post)
	}

	return posts, nil
}
