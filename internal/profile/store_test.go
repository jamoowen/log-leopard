package profile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreRoundTripStableIDAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "connections.json")
	s := NewStore(path)
	p, err := s.Save(Profile{Name: " Production ", ProjectID: "sample-project-123"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.ID) != 32 || p.Name != "Production" {
		t.Fatalf("unexpected profile: %#v", p)
	}
	p.Name = "Updated"
	updated, err := s.Save(p)
	if err != nil || updated.ID != p.ID {
		t.Fatalf("update: %#v %v", updated, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions: %v %v", info.Mode(), err)
	}
	items, err := s.List()
	if err != nil || len(items) != 1 || items[0] != updated {
		t.Fatalf("list: %#v %v", items, err)
	}
	if err := s.Delete(p.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestValidateRejectsNonProjectIdentifiers(t *testing.T) {
	for _, id := range []string{"123456", "Project_Name", "a", "projects/foo", "example.com:project"} {
		if err := Validate(Profile{Name: "x", ProjectID: id}); err == nil || !errors.Is(err, ErrValidation) {
			t.Errorf("accepted %q", id)
		}
	}
}
