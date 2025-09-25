package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

func sendPostRequest(client *http.Client, url string, data RegisterRequest, id int) {
	jsonData, _ := json.Marshal(data)

	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("Request %d: POST error: %v\n", id, err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Request %d: status %s\n", id, resp.Status)
}

func main() {
	url := "https://127.0.0.1/auth/register"
	requestsPerSecond := 100

	// Tạo HTTP client bỏ qua kiểm tra certificate
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}

	ticker := time.NewTicker(time.Second / time.Duration(requestsPerSecond))
	defer ticker.Stop()

	counter := 0
	for {
		<-ticker.C
		counter++
		data := RegisterRequest{
			Username: fmt.Sprintf("testuser%d", counter),
			Password: "123456",
			Email:    fmt.Sprintf("testuser%d@example.com", counter),
		}
		go sendPostRequest(client, url, data, counter)
	}
}
