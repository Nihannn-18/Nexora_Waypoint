package receipts

import (
	"context"
	"errors"
	"testing"
	"time"

	"waypoint.lk/api/internal/domain"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

var demoNow = time.Date(2026, 9, 26, 7, 50, 0, 0, time.FixedZone("Asia/Colombo", 5*3600+1800))

// fakeRepo is an in-memory Repository.
type fakeRepo struct {
	orders   map[string]OrderContext
	pods     map[string]*Pod
	receipts map[string]*Receipt
	created  []Receipt
}

func newFakeRepo() *fakeRepo {
	o := order()
	return &fakeRepo{
		orders:   map[string]OrderContext{o.OrderID: o},
		pods:     map[string]*Pod{o.OrderID: {Outcome: "DELIVERED", ReceiverName: "Ishara", PhotoRef: "pod/LEG1/abc", OccurredAt: demoNow}},
		receipts: map[string]*Receipt{},
	}
}

func (f *fakeRepo) OrderContext(_ context.Context, id string) (OrderContext, error) {
	o, ok := f.orders[id]
	if !ok {
		return OrderContext{}, ErrNotFound
	}
	return o, nil
}

func (f *fakeRepo) Pod(_ context.Context, id string) (*Pod, error) { return f.pods[id], nil }

func (f *fakeRepo) Receipt(_ context.Context, id string) (*Receipt, error) {
	return f.receipts[id], nil
}

func (f *fakeRepo) Create(_ context.Context, o OrderContext, rec Receipt) (Receipt, error) {
	if f.receipts[o.OrderID] != nil {
		return Receipt{}, ErrAlreadyReceived
	}
	rec.ReceiptID = "R1"
	f.receipts[o.OrderID] = &rec
	o.Status = string(domain.OrderReceived)
	f.orders[o.OrderID] = o
	f.created = append(f.created, rec)
	return rec, nil
}

func storeManager(outlet string) Scope { return Scope{Role: domain.RoleStoreManager, OutletID: outlet} }

func validInput() Input {
	return Input{
		Notes: "  one carton crushed  ",
		Lines: []LineInput{{OrderItemID: "L1", ReceivedQty: 7, DamagedQty: 1}, {OrderItemID: "L2", ReceivedQty: 5}},
	}
}

func TestServiceViewScope(t *testing.T) {
	tests := []struct {
		name    string
		scope   Scope
		wantErr error
	}{
		{name: "own outlet", scope: storeManager("OUT014")},
		{name: "dispatcher", scope: Scope{Role: domain.RoleDispatcher}},
		{name: "another outlet reads as not found", scope: storeManager("OUT015"), wantErr: ErrNotFound},
		{name: "store manager without an outlet", scope: storeManager(""), wantErr: ErrNotFound},
		{name: "driver", scope: Scope{Role: domain.RoleDriver}, wantErr: ErrNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(newFakeRepo(), fixedClock{demoNow})
			view, err := svc.View(context.Background(), "O1", tc.scope)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if view.Pod == nil || view.Pod.ReceiverName != "Ishara" {
				t.Errorf("pod = %+v, want the driver's POD", view.Pod)
			}
			if len(view.Order.Lines) != 2 || view.Order.Lines[0].LoaderFlag == nil {
				t.Errorf("lines = %+v, want the loader flag on L1", view.Order.Lines)
			}
		})
	}
}

func TestServiceSubmitRecordsReceipt(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, fixedClock{demoNow})

	rec, err := svc.Submit(context.Background(), "O1", "u-store", storeManager("OUT014"), validInput())
	if err != nil {
		t.Fatal(err)
	}
	if !rec.ReceivedAt.Equal(demoNow) {
		t.Errorf("receivedAt = %v, want the API clock %v", rec.ReceivedAt, demoNow)
	}
	if rec.ReceivedBy != "u-store" || rec.Notes != "one carton crushed" {
		t.Errorf("receipt = %+v", rec)
	}
	if rec.Status() != StatusReceivedWithIssue {
		t.Errorf("status = %q, want RECEIVED_WITH_ISSUE", rec.Status())
	}
	if repo.orders["O1"].Status != string(domain.OrderReceived) {
		t.Errorf("order status = %q, want RECEIVED", repo.orders["O1"].Status)
	}
}

func TestServiceSubmitRefusals(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		scope   Scope
		in      Input
		wantErr error
	}{
		{name: "in transit is not delivered yet", status: "IN_TRANSIT", scope: storeManager("OUT014"), in: validInput(), wantErr: ErrNotDelivered},
		{name: "failed has nothing to receive", status: "FAILED", scope: storeManager("OUT014"), in: validInput(), wantErr: ErrNotDelivered},
		{name: "received twice", status: "RECEIVED", scope: storeManager("OUT014"), in: validInput(), wantErr: ErrAlreadyReceived},
		{name: "another outlet", status: "DELIVERED", scope: storeManager("OUT015"), in: validInput(), wantErr: ErrNotFound},
		{name: "dispatcher cannot record a GRN", status: "DELIVERED", scope: Scope{Role: domain.RoleDispatcher}, in: validInput(), wantErr: ErrNotFound},
		{name: "invalid counts", status: "DELIVERED", scope: storeManager("OUT014"), in: Input{Lines: []LineInput{{OrderItemID: "L1", ReceivedQty: 9}, {OrderItemID: "L2", ReceivedQty: 5}}}, wantErr: ErrInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			o := repo.orders["O1"]
			o.Status = tc.status
			repo.orders["O1"] = o
			svc := NewService(repo, fixedClock{demoNow})

			_, err := svc.Submit(context.Background(), "O1", "u-store", tc.scope, tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if len(repo.created) != 0 {
				t.Fatalf("a refused GRN was written: %+v", repo.created)
			}
		})
	}
}

func TestServiceSubmitTwiceRecordsOnce(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, fixedClock{demoNow})
	if _, err := svc.Submit(context.Background(), "O1", "u-store", storeManager("OUT014"), validInput()); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Submit(context.Background(), "O1", "u-store", storeManager("OUT014"), validInput())
	if !errors.Is(err, ErrAlreadyReceived) {
		t.Fatalf("second submit err = %v, want ErrAlreadyReceived", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("receipts written = %d, want 1", len(repo.created))
	}
}
