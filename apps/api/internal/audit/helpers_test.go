package audit

import "time"

func mustTime(t interface{ Fatal(...any) }, s string) time.Time {
	tt, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return tt
}
