package bootstrap

import "testing"

func TestBackupMode(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test-only@localhost/postgres")
	t.Setenv("PUBLIC_ORIGIN", "https://assets.example.test")
	t.Setenv("APP_ENV", "production")
	t.Setenv("DISK_RESERVE_PERCENT", "10")
	for _, tc := range []struct {
		value             string
		disabled, invalid bool
	}{
		{"", false, false}, {"true", false, false}, {"false", true, false}, {"FALSE", false, true}, {"0", false, true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("BACKUP_ENABLED", tc.value)
			c, err := Read()
			if (err != nil) != tc.invalid || (!tc.invalid && c.BackupDisabled != tc.disabled) {
				t.Fatalf("mode=%q disabled=%v error=%v", tc.value, c.BackupDisabled, err)
			}
		})
	}
	t.Setenv("BACKUP_ENABLED", "false")
	t.Setenv("DISK_RESERVE_PERCENT", "0")
	if _, err := Read(); err == nil {
		t.Fatal("disabled backups bypassed production disk reserve")
	}
}
