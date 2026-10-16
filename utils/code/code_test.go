package code

import "testing"

// From the README's prefix table: a/A none, b/B .txt, c/C .md, d/D .log,
// e/E .json, f/F .yaml, g/G .csv, h/H .conf, i/I .sh, j/J .py, k/K .png,
// l/L .jpg, m/M .pdf, n/N .zip, o/O .gz.
func TestEncodeIDKnownExtensions(t *testing.T) {
	cases := []struct {
		idFull  string
		backend Backend
		want    string
	}{
		{"xyz123", Litterbox, "axyz123"},
		{"xyz123", Catbox, "Axyz123"},
		{"notes.txt", Litterbox, "bnotes"},
		{"notes.txt", Catbox, "Bnotes"},
		{"readme.md", Litterbox, "creadme"},
		{"app.log", Litterbox, "dapp"},
		{"data.json", Litterbox, "edata"},
		{"conf.yaml", Litterbox, "fconf"},
		{"table.csv", Litterbox, "gtable"},
		{"settings.conf", Litterbox, "hsettings"},
		{"run.sh", Litterbox, "irun"},
		{"main.py", Litterbox, "jmain"},
		{"logo.png", Catbox, "Klogo"},
		{"photo.jpg", Catbox, "Lphoto"},
		{"doc.pdf", Catbox, "Mdoc"},
		{"archive.zip", Catbox, "Narchive"},
		{"blob.gz", Catbox, "Oblob"},
	}
	for _, c := range cases {
		got := EncodeID(c.idFull, c.backend)
		if got != c.want {
			t.Errorf("EncodeID(%q, %q) = %q, want %q", c.idFull, c.backend, got, c.want)
		}
	}
}

func TestEncodeIDUnknownExtensionEscapes(t *testing.T) {
	if got, want := EncodeID("archive.tar", Litterbox), "0archive.tar"; got != want {
		t.Errorf("EncodeID = %q, want %q", got, want)
	}
	if got, want := EncodeID("archive.tar", Catbox), "1archive.tar"; got != want {
		t.Errorf("EncodeID = %q, want %q", got, want)
	}
}

func TestDecodeCodeRoundTrip(t *testing.T) {
	cases := []struct {
		idFull  string
		backend Backend
	}{
		{"xyz123", Litterbox},
		{"notes.txt", Catbox},
		{"archive.tar", Litterbox},
		{"archive.tar", Catbox},
	}
	for _, c := range cases {
		encoded := EncodeID(c.idFull, c.backend)
		backend, id, ok := DecodeCode(encoded)
		if !ok {
			t.Fatalf("DecodeCode(%q) failed", encoded)
		}
		if backend != c.backend || id != c.idFull {
			t.Errorf("DecodeCode(%q) = (%q, %q), want (%q, %q)",
				encoded, backend, id, c.backend, c.idFull)
		}
	}
}

func TestDecodeCodeInvalid(t *testing.T) {
	cases := []string{"", "!xyz", "pxyz"}
	for _, candidate := range cases {
		if _, _, ok := DecodeCode(candidate); ok {
			t.Errorf("DecodeCode(%q) should fail", candidate)
		}
	}
}
