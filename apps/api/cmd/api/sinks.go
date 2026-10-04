// Composition-root adapters. Two business packages (delivery, loading) each
// declare their own narrow AuditSink/NotifySink interfaces so they never import
// audit/notify. These adapters bridge those interfaces to the audit/notify
// transaction-bound writers, keeping the dependency direction inward.
package main

import (
	"context"

	"github.com/jackc/pgx/v5"

	"waypoint.lk/api/internal/audit"
	"waypoint.lk/api/internal/delivery"
	"waypoint.lk/api/internal/loading"
	"waypoint.lk/api/internal/notify"
	"waypoint.lk/api/internal/orders"
	"waypoint.lk/api/internal/routes"
)
// ordersAudit adapts audit.RecordTx to orders.AuditSink, so a queue close and
// its audit row commit in one transaction.
type ordersAudit struct{}

func (ordersAudit) RecordTx(ctx context.Context, tx pgx.Tx, e orders.AuditEvent) error {
	return audit.RecordTx(ctx, tx, e.Action, e.EntityType, e.EntityID, e.Actor, e.DepotID, e.OutletID, e.Result, e.Detail)
}

// routesAudit adapts audit.RecordTx to routes.AuditSink, so an allocation
// confirmation and its deferral trail commit in one transaction.
type routesAudit struct{}

func (routesAudit) RecordTx(ctx context.Context, tx pgx.Tx, e routes.AuditEvent) error {
	return audit.RecordTx(ctx, tx, e.Action, e.EntityType, e.EntityID, e.Actor, e.DepotID, e.OutletID, e.Result, e.Detail)
}

// deliveryAudit adapts audit.RecordTx to delivery.AuditSink.
type deliveryAudit struct{}

func (deliveryAudit) RecordTx(ctx context.Context, tx pgx.Tx, e delivery.AuditEvent) error {
	return audit.RecordTx(ctx, tx, e.Action, e.EntityType, e.EntityID, e.Actor, e.DepotID, e.OutletID, e.Result, e.Detail)
}

// deliveryNotify adapts notify.CreateForDispatchersTx to delivery.NotifySink.
type deliveryNotify struct{}

func (deliveryNotify) NotifyDispatchersTx(ctx context.Context, tx pgx.Tx, depotID, outletID, notifType, title, message, reference string) error {
	return notify.CreateForDispatchersTx(ctx, tx, depotID, outletID, notifType, title, message, reference)
}

// loadingAudit adapts audit.RecordTx to loading.AuditSink.
type loadingAudit struct{}

func (loadingAudit) RecordTx(ctx context.Context, tx pgx.Tx, e loading.AuditEvent) error {
	return audit.RecordTx(ctx, tx, e.Action, e.EntityType, e.EntityID, e.Actor, e.DepotID, e.OutletID, e.Result, e.Detail)
}

// loadingNotify adapts notify.CreateForDispatchersTx to loading.NotifySink.
type loadingNotify struct{}

func (loadingNotify) NotifyDispatchersTx(ctx context.Context, tx pgx.Tx, depotID, outletID, notifType, title, message, reference string) error {
	return notify.CreateForDispatchersTx(ctx, tx, depotID, outletID, notifType, title, message, reference)
}
