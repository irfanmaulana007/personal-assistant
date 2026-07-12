package imagegen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/irfanmaulana007/personal-assistant/server/internal/media"
)

// fakePNG is the decoded payload the API "returns" as base64.
var fakePNG = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a}

func testClient(srv *httptest.Server) *Client {
	return &Client{http: srv.Client(), baseURL: srv.URL}
}

func writeImage(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":  []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(fakePNG)}},
		"usage": map[string]int{"input_tokens": 12, "output_tokens": 34, "total_tokens": 46},
	})
}

func TestGenerate(t *testing.T) {
	var gotBody map[string]any
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		writeImage(w)
	}))
	defer srv.Close()

	res, err := testClient(srv).Generate(context.Background(), "sk-test", "a red fox", "1024x1024", "high")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if string(res.Image.Data) != string(fakePNG) {
		t.Errorf("image bytes = %v, want %v", res.Image.Data, fakePNG)
	}
	if res.Image.MimeType != "image/png" {
		t.Errorf("mime = %q, want image/png", res.Image.MimeType)
	}
	if res.Usage != (Usage{InputTokens: 12, OutputTokens: 34, TotalTokens: 46}) {
		t.Errorf("usage = %+v, want {12 34 46}", res.Usage)
	}
	if gotPath != "/images/generations" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotBody["model"] != Model || gotBody["prompt"] != "a red fox" ||
		gotBody["size"] != "1024x1024" || gotBody["quality"] != "high" {
		t.Errorf("request body = %v", gotBody)
	}
}

func TestEditSendsMultipartImage(t *testing.T) {
	var gotContentType, gotPrompt string
	var gotImageLen int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		gotPrompt = r.FormValue("prompt")
		if f, _, err := r.FormFile("image"); err == nil {
			b, _ := io.ReadAll(f)
			gotImageLen = len(b)
		} else {
			t.Errorf("FormFile image: %v", err)
		}
		writeImage(w)
	}))
	defer srv.Close()

	input := media.Image{MimeType: "image/png", Data: []byte("pretend-png-bytes")}
	res, err := testClient(srv).Edit(context.Background(), "sk-test", "add a hat", input, "", "")
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if string(res.Image.Data) != string(fakePNG) {
		t.Errorf("image bytes = %v, want %v", res.Image.Data, fakePNG)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data") {
		t.Errorf("content-type = %q, want multipart", gotContentType)
	}
	if gotPrompt != "add a hat" {
		t.Errorf("prompt = %q", gotPrompt)
	}
	if gotImageLen != len(input.Data) {
		t.Errorf("uploaded image len = %d, want %d", gotImageLen, len(input.Data))
	}
}

func TestGenerateSurfacesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"message": "billing hard limit reached"},
		})
	}))
	defer srv.Close()

	_, err := testClient(srv).Generate(context.Background(), "sk-test", "x", "", "")
	if err == nil || !strings.Contains(err.Error(), "billing hard limit reached") {
		t.Fatalf("err = %v, want to contain API message", err)
	}
}
