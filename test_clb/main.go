package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type FeedItem struct {
	ID          string `json:"id"`
	Message     string `json:"message"`
	CreatedTime string `json:"created_time"`
}

type Feed struct {
	Data   []FeedItem `json:"data"`
	Paging struct {
		Next string `json:"next"`
	} `json:"paging"`
}

type Response struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Feed Feed   `json:"feed"`
}

func fetchFeed(url string) (*Response, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data Response
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	return &data, nil
}

func main() {
	accessToken := "EAAlZCPmNqox8BPUw5WMenhZBYTJWK7iedOLT4rX46ym2McT3i0yvPjWA6XZCCTPn8RolEA7Gol2xzwSjCPSkCE5X5wmE018ODN4llLYU44yvL6oovVZAQafQHDCXh0aEBzAdDj5LvZA1fZC7e2AZBDZAKYmsV2JluZBfEUcVJ1o8b5nh8QTZCPZB3nG5m045ZCbFUZAQPZCboTGpR7bMk6HdZBB2D69NABjZA03RVWeve2EXomL2oVxOnOfEyKWgLAZDZD"
	url := fmt.Sprintf("https://graph.facebook.com/v23.0/me?fields=id,name,feed.limit(500)&access_token=%s", accessToken)

	result, err := fetchFeed(url)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// In ra thông tin page
	fmt.Println("Page ID:", result.ID)
	fmt.Println("Page Name:", result.Name)
	fmt.Println("Feed:")

	count := 0

	for _, item := range result.Feed.Data {
		fmt.Println("------------")
		fmt.Println("ID:", item.ID)
		fmt.Println("Time:", item.CreatedTime)
		fmt.Println("Message:", item.Message)
		count++
	}

	// Lưu ra file JSON UTF-8
	file, err := os.Create("feed.json")
	if err != nil {
		fmt.Println("Error creating file:", err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false) // quan trọng để giữ tiếng Việt
	if err := encoder.Encode(result); err != nil {
		fmt.Println("Error writing JSON:", err)
		return
	}

	fmt.Println("\nSaved feed to feed.json")
	fmt.Println("Count: ", count)
}
