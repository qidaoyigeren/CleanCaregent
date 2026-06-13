package mysql

import (
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
)

func TestUTCSessionDSNAddsTimeZone(t *testing.T) {
	normalized, err := utcSessionDSN(
		"root:password@tcp(127.0.0.1:3306)/cleancare?parseTime=true&loc=UTC",
	)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mysqldriver.ParseDSN(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Params["time_zone"]; got != "'+00:00'" {
		t.Fatalf("time_zone = %q", got)
	}
}

func TestUTCSessionDSNPreservesExplicitTimeZone(t *testing.T) {
	normalized, err := utcSessionDSN(
		"root:password@tcp(127.0.0.1:3306)/cleancare?parseTime=true&time_zone=%27%2B08%3A00%27",
	)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mysqldriver.ParseDSN(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Params["time_zone"]; got != "'+08:00'" {
		t.Fatalf("time_zone = %q", got)
	}
}
