package presentation

// Field is a native form control. Kind is text, password, number, integer, bool,
// select, combo, datetime or multiline; choices always keep display and IDs apart.
type Field struct {
	Key, Label, Kind, Help string
	Default                any
	Choices                []Choice
	Required               bool
	Min, Max               float64
}
type Action struct {
	ID, Title, Path, Kind, Confirm string
	Fields                         []Field
	Payload                        M
	Disabled                       bool
	Transform                      func(M) (M, error)
}

func (a Action) Body(values M) (M, error) {
	body := M{}
	for k, x := range a.Payload {
		body[k] = x
	}
	for k, x := range values {
		body[k] = x
	}
	if a.Transform != nil {
		return a.Transform(body)
	}
	return body, nil
}
