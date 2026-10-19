package httpapi

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUploadDeadlinesOverrideServerDefaults(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveUpload(w, r, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			// Both receiving the body and preparing the response outlive the
			// server's ordinary one-second deadlines.
			time.Sleep(1200 * time.Millisecond)
			_, _ = w.Write(data)
		}), 5*time.Second, 8*time.Second)
	}))
	server.Config.ReadTimeout = time.Second
	server.Config.WriteTimeout = time.Second
	server.Start()
	defer server.Close()
	reader, writer := io.Pipe()
	defer reader.Close()
	go func() {
		defer writer.Close()
		_, _ = writer.Write([]byte("a"))
		time.Sleep(1200 * time.Millisecond)
		_, _ = writer.Write([]byte("b"))
	}()
	client := server.Client()
	client.Timeout = 10 * time.Second
	response, err := client.Post(server.URL, "application/octet-stream", reader)
	if err != nil {
		t.Fatal("configured upload deadlines were not applied", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != 200 || string(data) != "ab" {
		t.Fatal(response.StatusCode, string(data), err)
	}
}

func TestUploadReadDeadlineInterruptsStalledBody(t *testing.T) {
	readError := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveUpload(w, r, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := io.ReadAll(r.Body)
			readError <- err
			w.WriteHeader(http.StatusRequestTimeout)
		}), 200*time.Millisecond, 3*time.Second)
	}))
	defer server.Close()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	go func() { _, _ = writer.Write([]byte("a")) }()
	client := server.Client()
	client.Timeout = 5 * time.Second
	response, err := client.Post(server.URL, "application/octet-stream", reader)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusRequestTimeout {
		t.Fatal(response.StatusCode)
	}
	select {
	case err := <-readError:
		var timeout net.Error
		if !errors.As(err, &timeout) || !timeout.Timeout() {
			t.Fatal("stalled body was not interrupted by a read deadline", err)
		}
	case <-time.After(time.Second):
		t.Fatal("missing upload read result")
	}
}

func TestUploadResponseDeadlineCancelsHandler(t *testing.T) {
	finished := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveUpload(w, r, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
			finished <- r.Context().Err()
			_, _ = w.Write([]byte("too late"))
		}), time.Second, 200*time.Millisecond)
	}))
	defer server.Close()
	client := server.Client()
	client.Timeout = 3 * time.Second
	response, err := client.Post(server.URL, "application/octet-stream", nil)
	if response != nil {
		response.Body.Close()
	}
	if err == nil {
		t.Fatal("response was written after its deadline")
	}
	select {
	case err := <-finished:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("response deadline did not cancel work")
	}
}
