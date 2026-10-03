package main

import (
	"context"
	"net/http"

	"waypoint.lk/api/internal/auth"
	"waypoint.lk/api/internal/authstore"
	"waypoint.lk/api/internal/media"
)

// mediaIdentityLoad adapts the real Better Auth identity loader to the media
// package's narrow loader signature. The media handler never sees a token; it
// only sees the resolved user, role and scope.
func mediaIdentityLoad(loader *auth.IdentityLoader) func(r *http.Request) (userID, role, depotID, outletID string, err error) {
	return func(r *http.Request) (string, string, string, string, error) {
		id, err := loader.Load(r.Context(), auth.HTTPRequestHeader(r))
		if err != nil {
			return "", "", "", "", err
		}
		return id.UserID, string(id.Role), id.DepotID, id.OutletID, nil
	}
}

// mediaOwnerReader adapts authstore's primitive scope lookups to media's
// OwnerReader, so the media package never imports SQL.
type mediaOwnerReader struct {
	store *authstore.Store
}

func (m mediaOwnerReader) OrderItemScope(ctx context.Context, orderItemID string) (media.OwnerScope, error) {
	depotID, outletID, found, err := m.store.OrderItemScope(ctx, orderItemID)
	if err != nil {
		return media.OwnerScope{}, err
	}
	return media.OwnerScope{DepotID: depotID, OutletID: outletID, Found: found}, nil
}

func (m mediaOwnerReader) LegScope(ctx context.Context, legID string) (media.OwnerScope, error) {
	depotID, outletID, found, err := m.store.LegScope(ctx, legID)
	if err != nil {
		return media.OwnerScope{}, err
	}
	return media.OwnerScope{DepotID: depotID, OutletID: outletID, Found: found}, nil
}
