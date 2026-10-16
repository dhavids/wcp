package backend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wcp/utils/code"
)

func TestUploadCatboxSendsExpectedFields(t *testing.T) {
	var gotReqtype, gotFilename string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		gotReqtype = r.FormValue("reqtype")
		if _, fh, err := r.FormFile("fileToUpload"); err == nil {
			gotFilename = fh.Filename
		}
		if r.FormValue("time") != "" {
			t.Error("catbox upload should not send a time field")
		}
		w.Write([]byte("https://files.catbox.moe/abc123.txt"))
	}))
	defer srv.Close()

	id, err := Upload(context.Background(), Options{Backend: code.Catbox, Host: srv.URL},
		"notes.txt", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if id != "abc123.txt" {
		t.Errorf("Upload() = %q, want %q", id, "abc123.txt")
	}
	if gotReqtype != "fileupload" {
		t.Errorf("reqtype = %q, want fileupload", gotReqtype)
	}
	if gotFilename != "notes.txt" {
		t.Errorf("filename = %q, want notes.txt", gotFilename)
	}
}

func TestUploadLitterboxSendsTimeField(t *testing.T) {
	var gotTime string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		gotTime = r.FormValue("time")
		w.Write([]byte("https://litter.catbox.moe/xyz789.txt"))
	}))
	defer srv.Close()

	id, err := Upload(context.Background(),
		Options{Backend: code.Litterbox, Host: srv.URL, LitterboxTTL: "12h"},
		"notes.txt", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if id != "xyz789.txt" {
		t.Errorf("Upload() = %q, want %q", id, "xyz789.txt")
	}
	if gotTime != "12h" {
		t.Errorf("time field = %q, want 12h", gotTime)
	}
}

func TestUploadNonURLReplyIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("No file!"))
	}))
	defer srv.Close()

	_, err := Upload(context.Background(), Options{Backend: code.Litterbox, Host: srv.URL},
		"notes.txt", strings.NewReader("hello"))
	if err == nil {
		t.Fatal("Upload should fail on a non-URL reply")
	}
	if !strings.Contains(err.Error(), "No file!") {
		t.Errorf("error = %q, want it to mention the reply", err)
	}
}

func TestRetrieveSuccessAndFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte("payload"))
	}))
	defer srv.Close()

	body, _, err := Retrieve(context.Background(), srv.URL+"/ok")
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if string(body) != "payload" {
		t.Errorf("Retrieve() = %q, want %q", body, "payload")
	}

	if _, _, err := Retrieve(context.Background(), srv.URL+"/missing"); err == nil {
		t.Fatal("Retrieve should fail on a 404")
	}
}

func TestStripHost(t *testing.T) {
	cases := map[string]string{
		"https://files.catbox.moe/abc123.txt": "abc123.txt",
		"http://litter.catbox.moe/xyz.txt":    "xyz.txt",
	}
	for in, want := range cases {
		if got := stripHost(in); got != want {
			t.Errorf("stripHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDescribeReply(t *testing.T) {
	if got := describeReply(""); got != "<empty response>" {
		t.Errorf("describeReply(\"\") = %q", got)
	}
	html := "<html><head><title>Oops</title></head><body>Request ID: <code>abc123</code></body></html>"
	if got := describeReply(html); got != "HTML error page - Oops (request id abc123)" {
		t.Errorf("describeReply(html) = %q", got)
	}
	long := strings.Repeat("x", 250)
	got := describeReply(long)
	if !strings.HasSuffix(got, "[250 bytes total]") {
		t.Errorf("describeReply(long) = %q, want truncation suffix", got)
	}
}
