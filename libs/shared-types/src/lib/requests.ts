/**
 * Request bodies, exactly as the Go handlers expect them.
 * Endpoint contracts are listed in docs/api.md.
 */

import type {
  Brand,
  DeliveryFailureReason,
  OrderStatus,
  TripNumber,
} from './domain';
import type {
  ClockTime,
  IsoDate,
  IsoDateTime,
  IsoWeek,
  LoadItemRecord,
  ProofOfDelivery,
  ReceiptIssue,
} from './entities';
import type { ConstraintCode, DeferralReasonType } from './constraints';

/* --- Auth ----------------------------------------------------------------- */

export interface LoginRequest {
  readonly email: string;
  readonly password: string;
}

/* --- Orders --------------------------------------------------------------- */

export interface CreateOrderRequest {
  readonly outletId: string;
  readonly requestedDeliveryDate: IsoDate;
  readonly items: readonly {
    readonly itemId: string;
    readonly quantity: number;
  }[];
  readonly notes?: string;
}

/** Query of GET /orders. Every filter is applied server-side. */
export interface ListOrdersQuery {
  readonly deliveryDate?: IsoDate;
  readonly status?: OrderStatus;
  readonly brand?: Brand;
  readonly depotId?: string;
  readonly district?: string;
  readonly outletId?: string;
  /** Matches the order number or outlet id, case-insensitively. */
  readonly search?: string;
  /** 1–200; the server defaults to 50. */
  readonly limit?: number;
  readonly offset?: number;
}

export interface CloseQueueRequest {
  readonly date: IsoDate;
  readonly depotId?: string;
  /**
   * The brands the dispatcher confirmed. Omit to close every brand. Closing is
   * per brand so one brand can be frozen while another keeps taking orders.
   */
  readonly brands?: readonly Brand[];
}

export interface OrderQueueQuery {
  readonly date: IsoDate;
  readonly depotId?: string;
  readonly brand?: Brand;
  readonly district?: string;
}

/* --- Planning ------------------------------------------------------------- */

export interface SuggestPlanRequest {
  readonly planningDate: IsoDate;
  readonly depotId: string;
}

export interface ValidateAllocationRequest {
  readonly orderId: string;
  readonly vehicleId: string;
  readonly tripNo: TripNumber;
  /** Omit when validating against the live plan; set to test a draft in progress. */
  readonly routeId?: string;
}

export interface RecalculateRouteRequest {
  readonly routeId: string;
  readonly routeVersion: number;
  readonly orderIds: readonly string[];
}

export interface ConfirmAllocationRequest {
  readonly jobId: string;
  readonly routes: readonly {
    readonly vehicleId: string;
    readonly tripNo: TripNumber;
    /** Stop order matters: the Loader loads in reverse of this sequence. */
    readonly orderIds: readonly string[];
  }[];
  /** Every unserved order must appear here with a reason. */
  readonly deferrals: readonly DeferralRequest[];
}

export interface DeferralRequest {
  readonly orderId: string;
  readonly reasonType: DeferralReasonType;
  readonly constraintCode?: ConstraintCode;
  readonly reasonText: string;
  readonly deferredToDate?: IsoDate;
}

export interface ReorderLegsRequest {
  readonly routeVersion: number;
  /** Leg ids in their new stop order. */
  readonly legIds: readonly string[];
}

/* --- Loading -------------------------------------------------------------- */

export interface RecordShortfallRequest {
  /** Server rejects any line where loaded + damaged + missing !== ordered. */
  readonly items: readonly Omit<LoadItemRecord, 'orderedQty'>[];
}

/* --- Delivery ------------------------------------------------------------- */

export interface DeliveryEventRequest {
  /** Generated on the device before the request, so retries are idempotent. */
  readonly clientEventId: string;
  readonly outcome: 'DELIVERED' | 'FAILED' | 'DELAYED';
  /** Device clock at capture time — preserved even when synced hours later. */
  readonly occurredAt: IsoDateTime;
  readonly createdOffline: boolean;
  readonly deliveredItems?: readonly {
    readonly orderItemId: string;
    readonly quantity: number;
    readonly damagedQty?: number;
    readonly shortQty?: number;
  }[];
  /** Required when outcome is FAILED. */
  readonly reasonCode?: DeliveryFailureReason;
  readonly notes?: string;
  readonly proofOfDelivery?: ProofOfDelivery;
}

export interface SyncEventsRequest {
  /** Uploaded in capture order; the server processes each independently. */
  readonly events: readonly (DeliveryEventRequest & {
    readonly legId: string;
  })[];
}

/* --- Receipt -------------------------------------------------------------- */

export interface CreateReceiptRequest {
  readonly status: 'RECEIVED' | 'RECEIVED_WITH_ISSUE' | 'REJECTED';
  readonly receivedItems: readonly {
    readonly orderItemId: string;
    readonly quantity: number;
  }[];
  readonly issues?: readonly ReceiptIssue[];
  readonly proof?: ProofOfDelivery;
}

/* --- Forecast ------------------------------------------------------------- */

export interface DemandForecastQuery {
  readonly depotId?: string;
  readonly brand?: Brand;
  readonly weekFrom?: IsoWeek;
  readonly weekTo?: IsoWeek;
}

/* --- Audit ---------------------------------------------------------------- */

/** Query of GET /audit. `from`/`to` are RFC 3339; `limit` ≤ 200. */
export interface AuditQuery {
  readonly actor?: string;
  readonly action?: string;
  readonly entityType?: string;
  readonly entityId?: string;
  readonly depotId?: string;
  readonly from?: IsoDateTime;
  readonly to?: IsoDateTime;
  readonly limit?: number;
  readonly offset?: number;
}

/* --- Live ----------------------------------------------------------------- */

export interface LiveRoutesQuery {
  readonly date?: IsoDate;
  readonly depotId?: string;
}

/** Shared by several queries that filter by a time-of-day range. */
export interface ClockRange {
  readonly from: ClockTime;
  readonly to: ClockTime;
}
