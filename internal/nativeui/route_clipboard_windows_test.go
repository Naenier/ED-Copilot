//go:build windows

package nativeui

import (
	"errors"
	"testing"
)

func TestRouteClipboardRetriesBusyAndAcknowledgesOnlySuccess(t *testing.T) {
	command := M{"id": "12", "text": "Next destination"}
	acks, writes := 0, 0
	last, err := deliverRouteClipboard("", command, func(string) error { writes++; return errors.New("busy") }, func(string) { acks++ })
	if err == nil || last != "" || acks != 0 {
		t.Fatal("failed write acknowledged", last, acks)
	}
	last, err = deliverRouteClipboard(last, command, func(text string) error {
		writes++
		if text != "Next destination" {
			t.Fatal(text)
		}
		return nil
	}, func(id string) {
		if id != "12" {
			t.Fatal(id)
		}
		acks++
	})
	if err != nil || last != "12" || acks != 1 {
		t.Fatal(last, err, acks)
	}
	last, err = deliverRouteClipboard(last, command, func(string) error { t.Fatal("duplicate clipboard write"); return nil }, func(string) { t.Fatal("duplicate ack") })
	if err != nil || writes != 2 {
		t.Fatal(err, writes)
	}
}

func TestRouteClipboardEmptyAndReplacement(t *testing.T) {
	for _, command := range []M{nil, {}, {"id": "1"}, {"text": "Name"}} {
		last, err := deliverRouteClipboard("old", command, func(string) error { t.Fatal("empty write"); return nil }, func(string) { t.Fatal("empty ack") })
		if err != nil || last != "old" {
			t.Fatal(last, err)
		}
	}
	last, err := deliverRouteClipboard("old", M{"id": "new", "text": "New destination"}, func(string) error { return nil }, func(string) {})
	if err != nil || last != "new" {
		t.Fatal(last, err)
	}
}
