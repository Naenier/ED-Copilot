package inara

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Error struct {
	Message, Code string
	Retryable     bool
	RetryAfter    time.Duration
}

func (e *Error) Error() string { return e.Message }

type Outcome struct {
	Index         int
	Status        int
	OK, Retryable bool
	Message       string
}
type BatchResult struct {
	HeaderStatus  int
	Results       []Outcome
	SetupRequired bool
	Message       string
}
type SendOptions struct {
	HTTPClient                                              *http.Client
	Endpoint, CommanderName, FrontierID, APIKey, AppVersion string
	Events                                                  []Event
	Now                                                     func() time.Time
	IsBeingDeveloped                                        bool
}

func safeMessage(raw, key string) string {
	s := str(raw)
	if key != "" {
		s = strings.ReplaceAll(s, key, "[скрыто]")
	}
	s = strings.Map(func(r rune) rune {
		if r < 32 {
			return ' '
		}
		return r
	}, s)
	r := []rune(s)
	if len(r) > 500 {
		r = r[:500]
	}
	return string(r)
}
func retryAfter(raw string, now time.Time) time.Duration {
	delay := time.Minute
	if seconds, err := strconv.ParseFloat(raw, 64); err == nil {
		delay = time.Duration(seconds * float64(time.Second))
	} else if at, err := http.ParseTime(raw); err == nil {
		delay = at.Sub(now)
	}
	if delay < 30*time.Second {
		delay = 30 * time.Second
	}
	if delay > 24*time.Hour {
		delay = 24 * time.Hour
	}
	return delay
}
func SendBatch(ctx context.Context, o SendOptions) (BatchResult, error) {
	result := BatchResult{Results: make([]Outcome, len(o.Events))}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.AppVersion == "" {
		o.AppVersion = "2.0.0"
	}
	if str(o.CommanderName) == "" {
		return result, &Error{Message: "Имя командира пока не прочитано из игры.", Code: "COMMANDER_REQUIRED"}
	}
	if !regexp.MustCompile(`^\d+\.\d+(?:\.\d+)?(?:[-+][\w.-]+)?$`).MatchString(o.AppVersion) {
		return result, &Error{Message: "Некорректная версия приложения для Inara.", Code: "VERSION_INVALID"}
	}
	if str(o.APIKey) == "" || len(o.APIKey) > 2048 || strings.ContainsAny(o.APIKey, "\r\n\x00") {
		return result, &Error{Message: "Укажите личный API-ключ Inara.", Code: "KEY_REQUIRED"}
	}
	if len(o.Events) > 100 {
		return result, &Error{Message: "В одном пакете Inara допустимо не более 100 событий.", Code: "BATCH_INVALID"}
	}
	outbound := []v.M{}
	indexes := []int{}
	for i, event := range o.Events {
		result.Results[i] = Outcome{Index: i, Status: 410, Message: "Событие устарело или не поддерживается API Inara."}
		if !Supported[event.Name] || !validTime(event.Timestamp, o.Now()) || event.Data == nil {
			continue
		}
		indexes = append(indexes, i)
		outbound = append(outbound, v.M{"eventName": event.Name, "eventTimestamp": event.Timestamp, "eventData": event.Data, "eventCustomID": i + 1})
	}
	if len(outbound) == 0 {
		return result, nil
	}
	header := v.M{"appName": "ED Copilot", "appVersion": o.AppVersion, "isBeingDeveloped": o.IsBeingDeveloped, "APIkey": o.APIKey, "commanderName": str(o.CommanderName)}
	if str(o.FrontierID) != "" {
		header["commanderFrontierID"] = str(o.FrontierID)
	}
	body, err := json.Marshal(v.M{"header": header, "events": outbound})
	if err != nil || len(body) > 1024*1024 {
		return result, &Error{Message: "Слишком большой пакет Inara.", Code: "BATCH_SIZE"}
	}
	if o.Endpoint == "" {
		o.Endpoint = Endpoint
	}
	client := o.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", o.Endpoint, bytes.NewReader(body))
	if err != nil {
		return result, &Error{Message: "Не удалось подготовить запрос Inara.", Code: "REQUEST"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ED Copilot/"+o.AppVersion)
	response, err := copyClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return result, &Error{Message: "Отправка Inara прервана; события сохранены в очереди.", Code: "CANCELLED", Retryable: true, RetryAfter: time.Minute}
		}
		return result, &Error{Message: "Не удалось связаться с Inara; повторим позже.", Code: "NETWORK", Retryable: true, RetryAfter: time.Minute}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		retry := response.StatusCode == 408 || response.StatusCode == 425 || response.StatusCode == 429 || response.StatusCode >= 500
		delay := time.Duration(0)
		if retry {
			delay = retryAfter(response.Header.Get("Retry-After"), o.Now())
		}
		return result, &Error{Message: fmt.Sprintf("Inara: HTTP %d. Проверьте ключ или повторите позже.", response.StatusCode), Code: fmt.Sprintf("HTTP_%d", response.StatusCode), Retryable: retry, RetryAfter: delay}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024+1))
	if err != nil || len(raw) > 2*1024*1024 {
		return result, &Error{Message: "Слишком большой или неполный ответ Inara.", Code: "RESPONSE_SIZE", Retryable: true}
	}
	var data v.M
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&data) != nil {
		return result, &Error{Message: "Некорректный ответ Inara; повторим позже.", Code: "RESPONSE_INVALID", Retryable: true}
	}
	status, ok := integer(v.At(data, "header", "eventStatus"))
	if !ok {
		return result, &Error{Message: "Ответ Inara не содержит статуса пакета.", Code: "RESPONSE_INVALID", Retryable: true}
	}
	result.HeaderStatus = int(status)
	result.Message = safeMessage(v.Str(v.At(data, "header", "eventStatusText")), o.APIKey)
	headerOK := status == 200 || status == 202
	result.SetupRequired = !headerOK && status == 400
	rows := v.Rows(data["events"])
	byID := map[int]v.M{}
	duplicates := map[int]bool{}
	for _, row := range rows {
		if id, valid := integer(row["eventCustomID"]); valid {
			key := int(id)
			if byID[key] != nil {
				duplicates[key] = true
			}
			byID[key] = row
		}
	}
	for i, index := range indexes {
		var row v.M
		if byID[index+1] != nil && !duplicates[index+1] {
			row = byID[index+1]
		} else if i < len(rows) && rows[i]["eventCustomID"] == nil {
			row = rows[i]
		}
		final, valid := integer(row["eventStatus"])
		if !headerOK {
			final = status
			valid = true
		}
		success := valid && (final == 200 || final == 202)
		retry := !valid || final >= 500 || final == 429
		message := result.Message
		if headerOK {
			message = safeMessage(v.String(row, "eventStatusText"), o.APIKey)
		}
		if !success && message == "" {
			message = "Не получено подтверждение события; повторим позже."
		}
		result.Results[index] = Outcome{Index: index, Status: int(final), OK: success, Retryable: retry, Message: message}
	}
	return result, nil
}
