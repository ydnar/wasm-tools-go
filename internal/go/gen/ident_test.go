package gen

import "testing"

func TestParseIdent(t *testing.T) {
	tests := []struct {
		s       string
		want    Ident
		wantErr bool
	}{
		{"io", Ident{"io", "io"}, false},
		{"io/fs", Ident{"io/fs", "fs"}, false},
		{"encoding/json", Ident{"encoding/json", "json"}, false},
		{"encoding/xml", Ident{"encoding/xml", "xml"}, false},
		{"encoding/xml#xml", Ident{"encoding/xml", "xml"}, false},
		{"encoding/xml#encxml", Ident{"encoding/xml", "encxml"}, false},
		{"encoding/xml#Encoder", Ident{"encoding/xml", "Encoder"}, false},
		{"encoding/xml#Decoder", Ident{"encoding/xml", "Decoder"}, false},
		{"wasi/clocks", Ident{"wasi/clocks", "clocks"}, false},
		{"wasi/clocks#clocks", Ident{"wasi/clocks", "clocks"}, false},
		{"wasi/clocks/wallclock", Ident{"wasi/clocks/wallclock", "wallclock"}, false},
		{"wasi/clocks/wallclock#wallclock", Ident{"wasi/clocks/wallclock", "wallclock"}, false},
		{"wasi/clocks/wallclock#DateTime", Ident{"wasi/clocks/wallclock", "DateTime"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got, err := ParseIdent(tt.s)
			if tt.wantErr && err == nil {
				t.Errorf("ParseIdent(%q): expected error, got nil error", tt.s)
			} else if !tt.wantErr && err != nil {
				t.Errorf("ParseIdent(%q): expected no error, got error: %v", tt.s, err)
			}
			if got != tt.want {
				t.Errorf("ParseIdent(%q): expected %v, got %v", tt.s, tt.want, got)
			}
		})
	}
}
