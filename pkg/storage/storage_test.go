package storage

import (
	"net/url"
	"testing"
)

func TestNormalizeConnectionString(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		want     string
		wantUser string
		wantPass string
	}{
		{
			name:     "plain password unchanged",
			in:       "postgres://user:password@localhost:5432/bbscope?sslmode=disable",
			want:     "postgres://user:password@localhost:5432/bbscope?sslmode=disable",
			wantUser: "user",
			wantPass: "password",
		},
		{
			name:     "hash and ampersand in password",
			in:       "postgres://unsealed:9CYSmk4VX7dcNNCk&#x!@localhost:5432/unsealed?sslmode=disable",
			want:     "postgres://unsealed:9CYSmk4VX7dcNNCk&%23x%21@localhost:5432/unsealed?sslmode=disable",
			wantUser: "unsealed",
			wantPass: "9CYSmk4VX7dcNNCk&#x!",
		},
		{
			name:     "slash, question mark, at and colon in password",
			in:       "postgresql://user:p/a?s@s:w@db.example.com/bbscope",
			want:     "postgresql://user:p%2Fa%3Fs%40s%3Aw@db.example.com/bbscope",
			wantUser: "user",
			wantPass: "p/a?s@s:w",
		},
		{
			name:     "already encoded password is not double encoded",
			in:       "postgres://user:p%23ss%2Fword@localhost/bbscope",
			want:     "postgres://user:p%23ss%2Fword@localhost/bbscope",
			wantUser: "user",
			wantPass: "p#ss/word",
		},
		{
			name:     "literal percent sign in password",
			in:       "postgres://user:100%@localhost/bbscope",
			want:     "postgres://user:100%25@localhost/bbscope",
			wantUser: "user",
			wantPass: "100%",
		},
		{
			name:     "user without password",
			in:       "postgres://user@localhost/bbscope",
			want:     "postgres://user@localhost/bbscope",
			wantUser: "user",
		},
		{
			name: "no credentials",
			in:   "postgres://localhost/bbscope",
			want: "postgres://localhost/bbscope",
		},
		{
			name: "key/value DSN unchanged",
			in:   "host=localhost user=user password=p#ss dbname=bbscope",
			want: "host=localhost user=user password=p#ss dbname=bbscope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeConnectionString(tt.in)
			if got != tt.want {
				t.Fatalf("NormalizeConnectionString(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if tt.wantUser == "" {
				return
			}
			u, err := url.Parse(got)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", got, err)
			}
			if u.User.Username() != tt.wantUser {
				t.Errorf("username = %q, want %q", u.User.Username(), tt.wantUser)
			}
			if pass, _ := u.User.Password(); pass != tt.wantPass {
				t.Errorf("password = %q, want %q", pass, tt.wantPass)
			}
		})
	}
}
