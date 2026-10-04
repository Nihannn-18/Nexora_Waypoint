package receipts

import (
	"errors"
	"testing"
)

func intPtr(n int) *int { return &n }

// order is a two-line order: L1 counted by the loader with 2 missing, L2 never
// counted.
func order() OrderContext {
	return OrderContext{
		OrderID: "O1", OrderNumber: "ORD-2026-000001", OutletID: "OUT014", DepotID: "D1", Status: "DELIVERED",
		Lines: []ExpectedLine{
			{OrderItemID: "L1", SKU: "FR-001", Name: "Milk", OrderedQty: 10, LoadedQty: intPtr(8), LoaderFlag: &LoaderFlag{MissingQty: 2}},
			{OrderItemID: "L2", SKU: "FR-002", Name: "Bread", OrderedQty: 5},
		},
	}
}

func TestExpectedQty(t *testing.T) {
	tests := []struct {
		name       string
		line       ExpectedLine
		wantQty    int
		wantSource string
	}{
		{name: "no loader count expects the order", line: ExpectedLine{OrderedQty: 10}, wantQty: 10, wantSource: ExpectedFromOrder},
		{name: "loader count wins", line: ExpectedLine{OrderedQty: 10, LoadedQty: intPtr(8)}, wantQty: 8, wantSource: ExpectedFromLoader},
		{name: "fully loaded", line: ExpectedLine{OrderedQty: 10, LoadedQty: intPtr(10)}, wantQty: 10, wantSource: ExpectedFromLoader},
		{name: "nothing loaded", line: ExpectedLine{OrderedQty: 10, LoadedQty: intPtr(0)}, wantQty: 0, wantSource: ExpectedFromLoader},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.line.ExpectedQty(); got != tc.wantQty {
				t.Errorf("ExpectedQty = %d, want %d", got, tc.wantQty)
			}
			if got := tc.line.ExpectedSource(); got != tc.wantSource {
				t.Errorf("ExpectedSource = %q, want %q", got, tc.wantSource)
			}
		})
	}
}

func TestLineConditionAndShort(t *testing.T) {
	tests := []struct {
		name      string
		line      Line
		wantShort int
		wantCond  string
	}{
		{name: "all received", line: Line{ExpectedQty: 8, ReceivedQty: 8}, wantShort: 0, wantCond: ConditionGood},
		{name: "one damaged", line: Line{ExpectedQty: 8, ReceivedQty: 7, DamagedQty: 1}, wantShort: 0, wantCond: ConditionDamaged},
		{name: "one short", line: Line{ExpectedQty: 8, ReceivedQty: 7}, wantShort: 1, wantCond: ConditionShort},
		{name: "damaged and short", line: Line{ExpectedQty: 8, ReceivedQty: 5, DamagedQty: 1}, wantShort: 2, wantCond: ConditionDamagedAndShort},
		{name: "nothing expected", line: Line{ExpectedQty: 0}, wantShort: 0, wantCond: ConditionGood},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.line.ShortQty(); got != tc.wantShort {
				t.Errorf("ShortQty = %d, want %d", got, tc.wantShort)
			}
			if got := tc.line.Condition(); got != tc.wantCond {
				t.Errorf("Condition = %q, want %q", got, tc.wantCond)
			}
		})
	}
}

func TestReceiptStatusAndIssues(t *testing.T) {
	clean := Receipt{Lines: []Line{{OrderItemID: "L1", ExpectedQty: 8, ReceivedQty: 8}}}
	if clean.Status() != StatusReceived || len(clean.Issues()) != 0 {
		t.Fatalf("clean receipt: status %q issues %v", clean.Status(), clean.Issues())
	}

	withIssue := Receipt{Lines: []Line{
		{OrderItemID: "L1", SKU: "FR-001", ExpectedQty: 8, ReceivedQty: 6, DamagedQty: 1},
		{OrderItemID: "L2", SKU: "FR-002", ExpectedQty: 5, ReceivedQty: 5},
	}}
	if withIssue.Status() != StatusReceivedWithIssue {
		t.Fatalf("status = %q, want %q", withIssue.Status(), StatusReceivedWithIssue)
	}
	issues := withIssue.Issues()
	if len(issues) != 2 {
		t.Fatalf("issues = %+v, want damaged and short on L1", issues)
	}
	if issues[0].Type != IssueDamaged || issues[0].Quantity != 1 || issues[0].OrderItemID != "L1" {
		t.Errorf("issue[0] = %+v", issues[0])
	}
	if issues[1].Type != IssueShort || issues[1].Quantity != 1 || issues[1].OrderItemID != "L1" {
		t.Errorf("issue[1] = %+v", issues[1])
	}
}

func TestBuildLines(t *testing.T) {
	tests := []struct {
		name      string
		in        Input
		wantErr   bool
		wantField string
	}{
		{
			name: "every line at expected passes",
			in:   Input{Lines: []LineInput{{OrderItemID: "L1", ReceivedQty: 8}, {OrderItemID: "L2", ReceivedQty: 5}}},
		},
		{
			name: "received + damaged exactly at expected passes",
			in:   Input{Lines: []LineInput{{OrderItemID: "L1", ReceivedQty: 7, DamagedQty: 1}, {OrderItemID: "L2", ReceivedQty: 5}}},
		},
		{
			name:    "one over the loader's count fails",
			in:      Input{Lines: []LineInput{{OrderItemID: "L1", ReceivedQty: 9}, {OrderItemID: "L2", ReceivedQty: 5}}},
			wantErr: true, wantField: "lines.L1",
		},
		{
			name:    "one over the ordered quantity on an uncounted line fails",
			in:      Input{Lines: []LineInput{{OrderItemID: "L1", ReceivedQty: 8}, {OrderItemID: "L2", ReceivedQty: 5, DamagedQty: 1}}},
			wantErr: true, wantField: "lines.L2",
		},
		{
			name:    "a missing line fails",
			in:      Input{Lines: []LineInput{{OrderItemID: "L1", ReceivedQty: 8}}},
			wantErr: true, wantField: "lines",
		},
		{
			name:    "an unknown line fails",
			in:      Input{Lines: []LineInput{{OrderItemID: "L1", ReceivedQty: 8}, {OrderItemID: "L2", ReceivedQty: 5}, {OrderItemID: "LX"}}},
			wantErr: true, wantField: "lines",
		},
		{
			name:    "a duplicated line fails",
			in:      Input{Lines: []LineInput{{OrderItemID: "L1", ReceivedQty: 8}, {OrderItemID: "L1", ReceivedQty: 8}}},
			wantErr: true, wantField: "lines[1].orderItemId",
		},
		{
			name:    "negative quantities fail",
			in:      Input{Lines: []LineInput{{OrderItemID: "L1", ReceivedQty: -1}, {OrderItemID: "L2", ReceivedQty: 5}}},
			wantErr: true, wantField: "lines[0]",
		},
		{
			name:    "no lines fails",
			in:      Input{},
			wantErr: true, wantField: "lines",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lines, err := BuildLines(order(), tc.in)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(lines) != 2 {
					t.Fatalf("lines = %d, want 2", len(lines))
				}
				if lines[0].ExpectedQty != 8 || lines[0].OrderedQty != 10 {
					t.Errorf("L1 expected/ordered = %d/%d, want 8/10", lines[0].ExpectedQty, lines[0].OrderedQty)
				}
				return
			}
			var ve ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want ValidationError", err)
			}
			if ve.Field != tc.wantField {
				t.Errorf("field = %q, want %q (%s)", ve.Field, tc.wantField, ve.Message)
			}
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("err does not wrap ErrInvalid")
			}
		})
	}
}

func TestBuildLinesPrefilledLoaderShortfallIsNotAnIssue(t *testing.T) {
	// The loader flagged 2 missing on L1; receiving the 8 that left the dock is a
	// clean receipt, not a new shortage.
	lines, err := BuildLines(order(), Input{Lines: []LineInput{{OrderItemID: "L1", ReceivedQty: 8}, {OrderItemID: "L2", ReceivedQty: 5}}})
	if err != nil {
		t.Fatal(err)
	}
	rec := Receipt{Lines: lines}
	if rec.Status() != StatusReceived {
		t.Fatalf("status = %q, want RECEIVED", rec.Status())
	}
}

func TestBuildLinesRejectsLongNotes(t *testing.T) {
	long := make([]byte, maxNotesLength+1)
	for i := range long {
		long[i] = 'x'
	}
	_, err := BuildLines(order(), Input{Notes: string(long), Lines: []LineInput{{OrderItemID: "L1", ReceivedQty: 8}, {OrderItemID: "L2", ReceivedQty: 5}}})
	var ve ValidationError
	if !errors.As(err, &ve) || ve.Field != "notes" {
		t.Fatalf("err = %v, want notes validation error", err)
	}
}
