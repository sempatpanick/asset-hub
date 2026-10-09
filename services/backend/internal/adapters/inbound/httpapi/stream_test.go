package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestActiveContentOutlivesOrdinaryResponseTimeout(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		controller := http.NewResponseController(w)
		stream := idleContentWriter{writer: w, controller: controller, timeout: time.Second}
		for range 5 {
			time.Sleep(60 * time.Millisecond)
			if _, err := stream.Write([]byte("chunk")); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		}
	}))
	server.Config.WriteTimeout = 100 * time.Millisecond
	server.Start()
	defer server.Close()
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || string(data) != strings.Repeat("chunk", 5) {
		t.Fatal("active stream was cut off", err, string(data))
	}
}
