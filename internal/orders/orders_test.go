package orders

import (
	"errors"
	"strings"
	"testing"
)

func TestOrderValidation(t *testing.T) {
	for _, input := range []string{"", " \t ", strings.Repeat("x", 256), "item\x00", string([]byte{0xff})} {
		if _, err := NormalizeItem(input); !errors.Is(err, ErrInvalidItem) {
			t.Fatal("invalid item accepted")
		}
	}
	if got, err := NormalizeItem("  example  "); err != nil || got != "example" {
		t.Fatal("item was not normalized")
	}
	if _, err := NormalizeItem(strings.Repeat("é", 255)); err != nil {
		t.Fatal("Unicode item limit must count characters")
	}
	if ValidateID("00000000-0000-4000-8000-000000000001") != nil {
		t.Fatal("canonical UUID rejected")
	}
	if ValidateID("x' OR 1=1") == nil || ValidateID("urn:uuid:00000000-0000-4000-8000-000000000001") == nil {
		t.Fatal("invalid identifier accepted")
	}
}

func TestListBounds(t *testing.T) {
	for _, limit := range []int{-1, 101, 1000000} {
		if _, err := (ListOptions{Limit: limit}).Validated(); !errors.Is(err, ErrInvalidPage) {
			t.Fatal("unbounded list accepted")
		}
	}
	options, err := (ListOptions{}).Validated()
	if err != nil || options.Limit != 20 {
		t.Fatal("default page size is not bounded")
	}
}
