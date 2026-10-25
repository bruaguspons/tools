package jdk

import "testing"

func TestParseJavaVersion(t *testing.T) {
	tests := []struct {
		name    string
		release string
		want    string
		wantErr string
	}{
		{
			name:    "quoted value",
			release: "IMPLEMENTOR=\"Eclipse\"\nJAVA_VERSION=\"21.0.12.1\"\nOS_NAME=\"Linux\"\n",
			want:    "21.0.12.1",
		},
		{
			name:    "missing line",
			release: "IMPLEMENTOR=\"Eclipse\"\n",
			wantErr: "no JAVA_VERSION line found",
		},
		{
			name:    "empty value",
			release: "JAVA_VERSION=\"\"\n",
			wantErr: "JAVA_VERSION is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseJavaVersion([]byte(tt.release))
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("ParseJavaVersion() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseJavaVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}
