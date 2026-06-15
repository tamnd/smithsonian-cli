package smithsonian

import (
	"testing"
)

// These tests are offline: they exercise the URI driver's pure string functions.
// The client's HTTP behaviour is covered in smithsonian_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "smithsonian" {
		t.Errorf("Scheme = %q, want smithsonian", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "smithsonian" {
		t.Errorf("Identity.Binary = %q, want smithsonian", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	typ, id, err := Domain{}.Classify("ld1-1646149545906-1646150180363-1")
	if err != nil {
		t.Fatalf("Classify error: %v", err)
	}
	if typ != "object" {
		t.Errorf("type = %q, want object", typ)
	}
	if id != "ld1-1646149545906-1646150180363-1" {
		t.Errorf("id = %q, want ld1-1646149545906-1646150180363-1", id)
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("object", "ld1-1646149545906-1646150180363-1")
	if err != nil {
		t.Fatalf("Locate error: %v", err)
	}
	want := "https://collections.si.edu/search/detail/ld1-1646149545906-1646150180363-1"
	if got != want {
		t.Errorf("Locate = %q, want %q", got, want)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "abc")
	if err == nil {
		t.Error("Locate with unknown type should return error")
	}
}
