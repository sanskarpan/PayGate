package reporting

import (
	"mime"
	"testing"
)

func TestNormalizeExportFormat(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		want  string
		valid bool
	}{
		{name: "empty defaults to csv", in: "", want: ExportFormatCSV, valid: true},
		{name: "whitespace defaults to csv", in: "   ", want: ExportFormatCSV, valid: true},
		{name: "csv passes through", in: "csv", want: ExportFormatCSV, valid: true},
		{name: "uppercase is normalized", in: "CSV", want: ExportFormatCSV, valid: true},
		{name: "padded is normalized", in: " Csv ", want: ExportFormatCSV, valid: true},
		{name: "xlsx is rejected", in: "xlsx", valid: false},
		{name: "xls is rejected", in: "xls", valid: false},
		{name: "json is rejected", in: "json", valid: false},
		{name: "path traversal is rejected", in: "../../etc/passwd", valid: false},
		{name: "quote injection is rejected", in: `csv"; filename="evil.exe`, valid: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := normalizeExportFormat(tc.in)
			if ok != tc.valid {
				t.Fatalf("normalizeExportFormat(%q) valid = %v, want %v", tc.in, ok, tc.valid)
			}
			if tc.valid && got != tc.want {
				t.Fatalf("normalizeExportFormat(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The export engine renders CSV unconditionally, so a job must never claim an
// extension the body does not match. This is the invariant the format
// allowlist exists to hold.
func TestExportFormatMatchesRenderedBody(t *testing.T) {
	format, ok := normalizeExportFormat("xlsx")
	if ok {
		t.Fatalf("xlsx was accepted as %q, but statementCSV is the only renderer", format)
	}
}

func TestContentDispositionEscapesTheFileName(t *testing.T) {
	// A quote in the name must not terminate the quoted string and introduce a
	// second parameter. Parsing the header back is the check that matters:
	// whatever we emit has to round-trip to exactly the name we put in.
	hostile := `report".csv"; filename="evil.exe`

	header := contentDisposition(hostile)

	mediaType, params, err := mime.ParseMediaType(header)
	if err != nil {
		t.Fatalf("emitted an unparseable Content-Disposition %q: %v", header, err)
	}
	if mediaType != "attachment" {
		t.Fatalf("media type = %q, want attachment", mediaType)
	}
	if got := params["filename"]; got != hostile {
		t.Fatalf("filename round-tripped as %q, want %q", got, hostile)
	}
	if len(params) != 1 {
		t.Fatalf("expected exactly one parameter, got %v", params)
	}
}

func TestContentDispositionOnAnOrdinaryName(t *testing.T) {
	header := contentDisposition("payments-20260101-20260131.csv")

	mediaType, params, err := mime.ParseMediaType(header)
	if err != nil {
		t.Fatalf("unparseable header %q: %v", header, err)
	}
	if mediaType != "attachment" {
		t.Fatalf("media type = %q, want attachment", mediaType)
	}
	if params["filename"] != "payments-20260101-20260131.csv" {
		t.Fatalf("filename = %q", params["filename"])
	}
}

func TestContentDispositionFallsBackWhenUnformattable(t *testing.T) {
	// mime.FormatMediaType refuses values it cannot represent and returns "".
	// The handler must still emit a valid header rather than an empty one.
	header := contentDisposition("\x7f\x00 invalid")
	if header == "" {
		t.Fatal("emitted an empty Content-Disposition")
	}
	if _, _, err := mime.ParseMediaType(header); err != nil {
		t.Fatalf("fallback header %q does not parse: %v", header, err)
	}
}
