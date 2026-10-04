package assignment

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"waypoint.lk/api/internal/domain"
)

// fakeStore is an in-memory Store for service tests. Writes record what was
// asked and mutate the maps so read-after-write behaves like the real store.
type fakeStore struct {
	drivers        []Driver
	users          map[string]User
	vehicleDepots  map[string]string
	outlets        map[string]bool
	vehicleAssign  map[string]VehicleAssignment // vehicleID|date
	driverAssign   map[string]VehicleAssignment // driverID|date
	managers       map[string]OutletManager     // outletID
	assignErr      error
	unassignErr    error
	outletQueryErr error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		drivers:       []Driver{{UserID: "u-driv", Name: "Kasun P.", Email: "kasun@example.com", DepotID: "d-pel"}},
		vehicleDepots: map[string]string{"VEH014": "d-pel", "VEH001": "d-pel", "VEH900": "d-kandy"},
		outlets:       map[string]bool{"OUT014": true, "OUT090": true},
		vehicleAssign: map[string]VehicleAssignment{},
		driverAssign:  map[string]VehicleAssignment{},
		managers:      map[string]OutletManager{},
		users: map[string]User{
			"u-driv":   {UserID: "u-driv", Name: "Kasun P.", Email: "kasun@example.com", Role: domain.RoleDriver, Active: true, DepotID: "d-pel"},
			"u-driv2":  {UserID: "u-driv2", Name: "Amal S.", Email: "amal@example.com", Role: domain.RoleDriver, Active: true, DepotID: "d-pel"},
			"u-load":   {UserID: "u-load", Name: "Nadeesha", Role: domain.RoleLoader, Active: true, DepotID: "d-pel"},
			"u-store":  {UserID: "u-store", Name: "Ishara S.", Email: "ishara@example.com", Role: domain.RoleStoreManager, Active: true, OutletID: "OUT014"},
			"u-store2": {UserID: "u-store2", Name: "Fathima", Email: "fathima@example.com", Role: domain.RoleStoreManager, Active: true},
			"u-off":    {UserID: "u-off", Name: "Off", Role: domain.RoleStoreManager, Active: false},
		},
	}
}

func (f *fakeStore) ListDrivers(context.Context, string) ([]Driver, error) {
	return f.drivers, nil
}

func (f *fakeStore) ListStoreManagers(context.Context) ([]ManagerCandidate, error) {
	out := []ManagerCandidate{}
	for _, u := range f.users {
		if u.Role == domain.RoleStoreManager && u.Active {
			out = append(out, ManagerCandidate{UserID: u.UserID, Name: u.Name, Email: u.Email, OutletID: u.OutletID})
		}
	}
	return out, nil
}

func (f *fakeStore) UserByID(_ context.Context, userID string) (User, error) {
	u, ok := f.users[userID]
	if !ok {
		return User{}, fmt.Errorf("%w: user", ErrNotFound)
	}
	return u, nil
}

func (f *fakeStore) VehicleDepot(_ context.Context, vehicleID string) (string, error) {
	d, ok := f.vehicleDepots[vehicleID]
	if !ok {
		return "", fmt.Errorf("%w: vehicle", ErrNotFound)
	}
	return d, nil
}

func (f *fakeStore) OutletExists(_ context.Context, outletID string) (bool, error) {
	if f.outletQueryErr != nil {
		return false, f.outletQueryErr
	}
	return f.outlets[outletID], nil
}

func (f *fakeStore) AssignmentForVehicle(_ context.Context, vehicleID, date string) (VehicleAssignment, bool, error) {
	a, ok := f.vehicleAssign[vehicleID+"|"+date]
	return a, ok, nil
}

func (f *fakeStore) AssignmentForDriver(_ context.Context, driverID, date string) (VehicleAssignment, bool, error) {
	a, ok := f.driverAssign[driverID+"|"+date]
	return a, ok, nil
}

func (f *fakeStore) AssignVehicle(_ context.Context, a VehicleAssignment, _ string) (VehicleAssignment, error) {
	if f.assignErr != nil {
		return VehicleAssignment{}, f.assignErr
	}
	f.vehicleAssign[a.VehicleID+"|"+a.AssignmentDate] = a
	f.driverAssign[a.Driver.UserID+"|"+a.AssignmentDate] = a
	return a, nil
}

func (f *fakeStore) UnassignVehicle(_ context.Context, vehicleID, date, _ string) (VehicleAssignment, error) {
	if f.unassignErr != nil {
		return VehicleAssignment{}, f.unassignErr
	}
	a, ok := f.vehicleAssign[vehicleID+"|"+date]
	if !ok {
		return VehicleAssignment{}, fmt.Errorf("%w: assignment", ErrNotFound)
	}
	delete(f.vehicleAssign, vehicleID+"|"+date)
	delete(f.driverAssign, a.Driver.UserID+"|"+date)
	return a, nil
}

func (f *fakeStore) ManagerForOutlet(_ context.Context, outletID string) (OutletManager, bool, error) {
	m, ok := f.managers[outletID]
	return m, ok, nil
}

func (f *fakeStore) SetManager(_ context.Context, outletID, userID, _ string) (OutletManager, error) {
	for id, m := range f.managers {
		if m.UserID == userID {
			delete(f.managers, id)
		}
	}
	u := f.users[userID]
	m := OutletManager{OutletID: outletID, UserID: userID, Name: u.Name, Email: u.Email, DepotID: u.DepotID}
	f.managers[outletID] = m
	return m, nil
}

func (f *fakeStore) ClearManager(_ context.Context, outletID, _ string) (OutletManager, error) {
	m, ok := f.managers[outletID]
	if !ok {
		return OutletManager{}, fmt.Errorf("%w: manager", ErrNotFound)
	}
	delete(f.managers, outletID)
	return m, nil
}

func newService() (*Service, *fakeStore) {
	s := newFakeStore()
	return NewService(s, nil), s
}

func TestAssignDriverValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("valid assignment succeeds", func(t *testing.T) {
		svc, _ := newService()
		a, err := svc.AssignDriver(ctx, AssignDriverInput{VehicleID: "VEH014", DriverID: "u-driv", Date: "2026-09-26"}, "u-disp")
		if err != nil {
			t.Fatalf("assign: %v", err)
		}
		if a.VehicleID != "VEH014" || a.Driver.UserID != "u-driv" || a.AssignmentDate != "2026-09-26" || a.DepotID != "d-pel" {
			t.Fatalf("assignment = %+v", a)
		}
	})

	t.Run("non-driver is rejected", func(t *testing.T) {
		svc, _ := newService()
		_, err := svc.AssignDriver(ctx, AssignDriverInput{VehicleID: "VEH014", DriverID: "u-load", Date: "2026-09-26"}, "u-disp")
		assertValidation(t, err, "driverId")
	})

	t.Run("inactive driver is rejected", func(t *testing.T) {
		svc, _ := newService()
		_, err := svc.AssignDriver(ctx, AssignDriverInput{VehicleID: "VEH014", DriverID: "u-off", Date: "2026-09-26"}, "u-disp")
		assertValidation(t, err, "driverId")
	})

	t.Run("depot mismatch is rejected", func(t *testing.T) {
		svc, _ := newService()
		_, err := svc.AssignDriver(ctx, AssignDriverInput{VehicleID: "VEH900", DriverID: "u-driv", Date: "2026-09-26"}, "u-disp")
		assertValidation(t, err, "driverId")
	})

	t.Run("bad date is rejected", func(t *testing.T) {
		svc, _ := newService()
		_, err := svc.AssignDriver(ctx, AssignDriverInput{VehicleID: "VEH014", DriverID: "u-driv", Date: "26 Sep"}, "u-disp")
		assertValidation(t, err, "date")
	})

	t.Run("a driver already on another vehicle the same date conflicts", func(t *testing.T) {
		svc, store := newService()
		if _, err := svc.AssignDriver(ctx, AssignDriverInput{VehicleID: "VEH014", DriverID: "u-driv", Date: "2026-09-26"}, "u-disp"); err != nil {
			t.Fatal(err)
		}
		_, err := svc.AssignDriver(ctx, AssignDriverInput{VehicleID: "VEH001", DriverID: "u-driv", Date: "2026-09-26"}, "u-disp")
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("err = %v, want ErrConflict", err)
		}
		_ = store
	})

	t.Run("changing a vehicle's driver on the same date is allowed", func(t *testing.T) {
		svc, _ := newService()
		if _, err := svc.AssignDriver(ctx, AssignDriverInput{VehicleID: "VEH014", DriverID: "u-driv", Date: "2026-09-26"}, "u-disp"); err != nil {
			t.Fatal(err)
		}
		a, err := svc.AssignDriver(ctx, AssignDriverInput{VehicleID: "VEH014", DriverID: "u-driv2", Date: "2026-09-26"}, "u-disp")
		if err != nil || a.Driver.UserID != "u-driv2" {
			t.Fatalf("reassign = %+v, %v", a, err)
		}
	})
}

func TestUnassignDriver(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService()
	if _, err := svc.AssignDriver(ctx, AssignDriverInput{VehicleID: "VEH014", DriverID: "u-driv", Date: "2026-09-26"}, "u-disp"); err != nil {
		t.Fatal(err)
	}
	removed, err := svc.UnassignDriver(ctx, "VEH014", "2026-09-26", "u-disp")
	if err != nil || removed.Driver.UserID != "u-driv" {
		t.Fatalf("remove = %+v, %v", removed, err)
	}
	if _, err := svc.UnassignDriver(ctx, "VEH014", "2026-09-26", "u-disp"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second remove = %v, want ErrNotFound", err)
	}
}

func TestAssignManagerValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("store manager is assigned", func(t *testing.T) {
		svc, _ := newService()
		m, err := svc.AssignManager(ctx, AssignManagerInput{OutletID: "OUT014", UserID: "u-store2"}, "u-disp")
		if err != nil || m.UserID != "u-store2" || m.OutletID != "OUT014" {
			t.Fatalf("manager = %+v, %v", m, err)
		}
	})

	t.Run("non-store-manager is rejected", func(t *testing.T) {
		svc, _ := newService()
		_, err := svc.AssignManager(ctx, AssignManagerInput{OutletID: "OUT014", UserID: "u-driv"}, "u-disp")
		assertValidation(t, err, "userId")
	})

	t.Run("inactive user is rejected", func(t *testing.T) {
		svc, _ := newService()
		_, err := svc.AssignManager(ctx, AssignManagerInput{OutletID: "OUT014", UserID: "u-off"}, "u-disp")
		assertValidation(t, err, "userId")
	})

	t.Run("unknown outlet is not found", func(t *testing.T) {
		svc, _ := newService()
		_, err := svc.AssignManager(ctx, AssignManagerInput{OutletID: "OUT999", UserID: "u-store"}, "u-disp")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("reassigning moves the manager off the previous outlet", func(t *testing.T) {
		svc, store := newService()
		if _, err := svc.AssignManager(ctx, AssignManagerInput{OutletID: "OUT014", UserID: "u-store"}, "u-disp"); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.AssignManager(ctx, AssignManagerInput{OutletID: "OUT090", UserID: "u-store"}, "u-disp"); err != nil {
			t.Fatal(err)
		}
		if _, ok := store.managers["OUT014"]; ok {
			t.Fatal("manager still on OUT014 after moving to OUT090")
		}
		if m := store.managers["OUT090"]; m.UserID != "u-store" {
			t.Fatalf("OUT090 manager = %+v", m)
		}
	})
}

func TestOutletManagerAndClear(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService()
	if _, err := svc.AssignManager(ctx, AssignManagerInput{OutletID: "OUT014", UserID: "u-store"}, "u-disp"); err != nil {
		t.Fatal(err)
	}
	m, ok, err := svc.OutletManager(ctx, "OUT014")
	if err != nil || !ok || m.UserID != "u-store" {
		t.Fatalf("manager = %+v %v %v", m, ok, err)
	}
	if _, err := svc.UnassignManager(ctx, "OUT014", "u-disp"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, ok, _ := svc.OutletManager(ctx, "OUT014"); ok {
		t.Fatal("manager still present after clear")
	}
}

func assertValidation(t *testing.T, err error, field string) {
	t.Helper()
	var ve ValidationError
	if !errors.As(err, &ve) || ve.Field != field {
		t.Fatalf("err = %v, want ValidationError on %q", err, field)
	}
}
