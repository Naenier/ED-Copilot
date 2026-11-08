package inara

import (
	v "github.com/Naenier/ED-Copilot/internal/value"
	"testing"
	"time"
)

func TestCAPIStatusIdentityMustMatchBoundCommanderEvenWithSameName(t *testing.T) {
	encrypt, decrypt := cryptFixture(t)
	clock, state := clockFixture(), newStateFixture()
	m, err := New(Options{DataDir: t.TempDir(), Encrypt: encrypt, Decrypt: decrypt, Now: clock.Now, GetState: state.Get, AutoDelay: time.Hour, Send: ackAll})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err = m.Configure(v.M{"apiKey": "synthetic-key", "enabled": true}); err != nil {
		t.Fatal(err)
	}
	result := v.M{"status": v.M{"connected": true, "fid": "OTHER-FID"}, "metadata": v.M{"source": "Frontier CAPI", "observedAt": v.M{"profile": v.ISO(clock.Now())}}, "data": v.M{"profile": v.M{"commander": v.M{"name": "Synthetic Pilot", "credits": 123456, "debt": 0}}}}
	m.ObserveCAPI(result)
	if v.Int(m.Status()["queued"]) != 0 {
		t.Fatal("same-name foreign account was queued")
	}
	v.Obj(result["status"])["fid"] = "FSYNTHETIC"
	v.Obj(result["status"])["commanderMismatch"] = true
	m.ObserveCAPI(result)
	if v.Int(m.Status()["queued"]) != 0 {
		t.Fatal("mismatched account was queued")
	}
	v.Obj(result["status"])["commanderMismatch"] = false
	m.ObserveCAPI(result)
	if v.Int(m.Status()["queued"]) != 1 {
		t.Fatal("verified current account was not queued")
	}
}
