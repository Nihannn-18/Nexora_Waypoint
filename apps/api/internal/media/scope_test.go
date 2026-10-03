package media

import (
	"context"
	"errors"
	"testing"
)

type fakeOwnerReader struct {
	order OwnerScope
	leg   OwnerScope
	err   error
}

func (f fakeOwnerReader) OrderItemScope(context.Context, string) (OwnerScope, error) {
	return f.order, f.err
}

func (f fakeOwnerReader) LegScope(context.Context, string) (OwnerScope, error) {
	return f.leg, f.err
}

func TestScopeAuthorizerUpload(t *testing.T) {
	reader := fakeOwnerReader{
		order: OwnerScope{DepotID: "d-peli", OutletID: "OUT014", Found: true},
		leg:   OwnerScope{DepotID: "d-peli", OutletID: "OUT014", Found: true},
	}
	a := NewScopeAuthorizer(reader)

	cases := []struct {
		name    string
		p       Principal
		purpose Purpose
		owner   string
		wantErr bool
	}{
		{"loader shortfall own depot", Principal{UserID: "u", Role: roleLoader, DepotID: "d-peli"}, PurposeShortfall, "oi-1", false},
		{"driver pod own depot", Principal{UserID: "u", Role: roleDriver, DepotID: "d-peli"}, PurposePOD, "leg-1", false},
		{"loader pod wrong purpose role", Principal{UserID: "u", Role: roleLoader, DepotID: "d-peli"}, PurposePOD, "leg-1", true},
		{"driver shortfall wrong purpose role", Principal{UserID: "u", Role: roleDriver, DepotID: "d-peli"}, PurposeShortfall, "oi-1", true},
		{"cross depot shortfall", Principal{UserID: "u", Role: roleLoader, DepotID: "d-kandy"}, PurposeShortfall, "oi-1", true},
		{"cross depot pod", Principal{UserID: "u", Role: roleDriver, DepotID: "d-kandy"}, PurposePOD, "leg-1", true},
		{"dispatcher cannot upload", Principal{UserID: "u", Role: roleDispatcher, DepotID: "d-peli"}, PurposePOD, "leg-1", true},
		{"store manager cannot upload", Principal{UserID: "u", Role: roleStoreManager, DepotID: "d-peli"}, PurposeShortfall, "oi-1", true},
		{"missing depot scope", Principal{UserID: "u", Role: roleLoader}, PurposeShortfall, "oi-1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := a.AuthorizeUpload(context.Background(), tc.p, tc.purpose, tc.owner)
			if tc.wantErr && !errors.Is(err, ErrForbidden) {
				t.Fatalf("err = %v, want ErrForbidden", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
		})
	}

	t.Run("unknown owner is forbidden", func(t *testing.T) {
		a := NewScopeAuthorizer(fakeOwnerReader{order: OwnerScope{Found: false}})
		err := a.AuthorizeUpload(context.Background(), Principal{Role: roleLoader, DepotID: "d-peli"}, PurposeShortfall, "missing")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestScopeAuthorizerRead(t *testing.T) {
	a := NewScopeAuthorizer(fakeOwnerReader{
		leg: OwnerScope{DepotID: "d-peli", OutletID: "OUT014", Found: true},
	})

	cases := []struct {
		name    string
		p       Principal
		wantErr bool
	}{
		{"dispatcher reads any depot", Principal{Role: roleDispatcher, DepotID: "d-peli"}, false},
		{"loader same depot", Principal{Role: roleLoader, DepotID: "d-peli"}, false},
		{"loader other depot", Principal{Role: roleLoader, DepotID: "d-kandy"}, true},
		{"driver same depot", Principal{Role: roleDriver, DepotID: "d-peli"}, false},
		{"store manager own outlet", Principal{Role: roleStoreManager, OutletID: "OUT014"}, false},
		{"store manager other outlet", Principal{Role: roleStoreManager, OutletID: "OUT099"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := a.AuthorizeRead(context.Background(), tc.p, "pod/leg-1/obj")
			if tc.wantErr && !errors.Is(err, ErrForbidden) {
				t.Fatalf("err = %v, want ErrForbidden", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
		})
	}

	t.Run("malformed key is forbidden", func(t *testing.T) {
		err := a.AuthorizeRead(context.Background(), Principal{Role: roleDispatcher}, "nope")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestParseKey(t *testing.T) {
	purpose, owner, err := ParseKey("pod/leg-1/abc")
	if err != nil || purpose != PurposePOD || owner != "leg-1" {
		t.Fatalf("ParseKey = %v, %q, %v", purpose, owner, err)
	}
	if _, _, err := ParseKey("bogus/leg-1/abc"); err == nil {
		t.Fatal("expected error for unknown purpose")
	}
	if _, _, err := ParseKey("pod/leg-1"); err == nil {
		t.Fatal("expected error for malformed key")
	}
}
