package web

import (
	"testing"
	"time"
)

func TestNilLocationScheduleDoesNotPanic(t *testing.T) {
	at := time.Date(2026, 9, 28, 15, 4, 0, 0, time.UTC)
	got := scheduleDate(at, nil)
	if got.Location() == nil {
		t.Fatal("scheduleDate(nil location) returned a nil location")
	}
	srv := &Server{cfg: Config{Clock: func() time.Time { return at }}}
	parsed, err := srv.parseScheduleLocal("2026-09-28T15:04")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.IsZero() {
		t.Fatal("parseScheduleLocal with a nil location returned a zero time")
	}
	if locationName(srv.location(), at) == "" {
		t.Fatal("location name was empty")
	}
}
