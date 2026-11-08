package app

import (
	"errors"
	"github.com/Naenier/ED-Copilot/internal/localization"
	"math"

	"github.com/Naenier/ED-Copilot/internal/systems"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func (e *Engine) Language() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return localization.Normalize(v.Str(e.settings["language"]))
}

func systemSequence(input M) (uint64, error) {
	n, ok := v.Number(input["sequence"])
	if !ok || n < 0 || n > 9007199254740991 || math.Trunc(n) != n {
		return 0, errors.New("Некорректный номер запроса систем")
	}
	return uint64(n), nil
}

// QuerySystems is a read-only RPC. It deliberately does not enter the domain
// snapshot lock, Notify, Save or any full-history search path for each keystroke.
func (e *Engine) QuerySystems(input M) (M, error) {
	seq, err := systemSequence(input)
	if err != nil {
		return nil, err
	}
	request := systems.Request{Query: v.Str(input["query"]), FieldID: v.Str(input["fieldId"]), Sequence: seq, Limit: v.Int(input["limit"])}
	if e.opts.DisableNetwork || e.Systems == nil {
		return M{"query": request.Query, "fieldId": request.FieldID, "sequence": request.Sequence, "status": "disabled", "minChars": systems.MinChars, "source": "Spansh", "suggestions": []M{}, "errorCode": "network-disabled"}, nil
	}
	result, err := e.Systems.Query(e.ctx, request)
	// Expected network/provider errors use a bounded code in the result. Do not
	// surface response bodies, credentials, or a blocking application-wide toast.
	if err != nil && result.ErrorCode == "" {
		return nil, err
	}
	return v.Obj(v.Clone(result)), nil
}

func (e *Engine) CancelSystems(input M) (M, error) {
	seq, err := systemSequence(input)
	if err != nil {
		return nil, err
	}
	if e.Systems != nil {
		e.Systems.Cancel(v.Str(input["fieldId"]), seq)
	}
	return M{"ok": true, "fieldId": input["fieldId"], "sequence": seq}, nil
}
