package update

import "testing"

func TestIsUpgradable(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
		wantErr bool
	}{
		{name: "dev build", current: "dev", latest: "v1.2.3", want: true},
		{name: "patch update", current: "v1.2.2", latest: "v1.2.3", want: true},
		{name: "same version", current: "v1.2.3", latest: "v1.2.3"},
		{name: "older latest", current: "v1.2.4", latest: "v1.2.3"},
		{name: "release beats prerelease", current: "v1.2.3-rc.1", latest: "v1.2.3", want: true},
		{name: "prerelease order", current: "v1.2.3-rc.1", latest: "v1.2.3-rc.2", want: true},
		{name: "invalid current", current: "1.2.3", latest: "v1.2.3", wantErr: true},
		{name: "invalid latest", current: "v1.2.3", latest: "latest", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsUpgradable(tt.current, tt.latest)
			if tt.wantErr {
				if err == nil {
					t.Fatal("IsUpgradable() expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("IsUpgradable() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("IsUpgradable() = %v, want %v", got, tt.want)
			}
		})
	}
}
