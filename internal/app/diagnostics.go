package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Naenier/ED-Copilot/internal/diagnostics"
	"github.com/Naenier/ED-Copilot/internal/inara"
	"github.com/Naenier/ED-Copilot/internal/journal"
	"github.com/Naenier/ED-Copilot/internal/localization"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const diagnosticSeparator = " · "

func (e *Engine) recordJournalPoll(report journal.PollReport) {
	if report.Failed {
		if report.ErrorChanged {
			e.diagnostics.Log(diagnostics.Error, "journal", "read", "Не удалось прочитать журналы игры")
		}
		return
	}
	if report.Scan {
		message := fmt.Sprintf("Журналы игры прочитаны%sФайлы %d/%d%sСобытия %d%s%d ms", diagnosticSeparator, report.FilesRead,
			report.TotalFiles, diagnosticSeparator, report.Events, diagnosticSeparator, report.Elapsed.Milliseconds())
		e.diagnostics.Log(diagnostics.Info, "journal", "scan", message)
	} else if report.ErrorChanged {
		e.diagnostics.Log(diagnostics.Info, "journal", "read", "Чтение журналов игры восстановлено")
	}
}

func (e *Engine) recordLiveJournalEvent(kind string) {
	var subsystem string
	switch kind {
	case "LoadGame", "Shutdown":
		subsystem = "journal"
	case "FSDJump", "NavRoute", "NavRouteClear":
		subsystem = "exploration"
	case "CarrierJump", "CarrierJumpRequest", "CarrierJumpCancelled", "CarrierDepositFuel":
		subsystem = "carrier"
	case "MarketBuy", "MarketSell":
		subsystem = "trade"
	case "EngineerCraft", "EngineerProgress", "MaterialTrade":
		subsystem = "engineering"
	case "ColonisationConstructionDepot", "ColonisationContribution":
		subsystem = "colony"
	default:
		return
	}
	e.diagnostics.Log(diagnostics.Debug, subsystem, "journal."+strings.ToLower(kind), "Обработано событие игры")
}

func (e *Engine) diagnosticLogs(input M) (M, error) {
	language := e.Language()
	displayMessage := func(entry diagnostics.Entry) string {
		label, details, ok := strings.Cut(entry.Message, diagnosticSeparator)
		label = localization.Text(language, label)
		if language == "en" && entry.Event == "scan" && entry.Subsystem == "journal" {
			details = strings.ReplaceAll(strings.ReplaceAll(details, "Файлы ", "Files "), "События ", "Events ")
		}
		if ok {
			return label + diagnosticSeparator + details
		}
		return label
	}
	page, err := e.diagnostics.Query(diagnostics.Filter{
		Level: diagnostics.Level(v.Str(input["level"])), Subsystem: v.Str(input["subsystem"]),
		Search: v.Str(input["search"]), Before: v.Str(input["before"]), Limit: v.Int(input["limit"]),
		MatchText: displayMessage,
	})
	if err != nil {
		return nil, err
	}
	for i, entry := range page.Entries {
		page.Entries[i].Message = displayMessage(entry)
	}
	return M{"entries": page.Entries, "next": page.Next, "subsystems": page.Subsystems}, nil
}

func (e *Engine) recordCommand(path, action string, started time.Time, result M, err error) {
	event := strings.ReplaceAll(strings.TrimPrefix(strings.TrimPrefix(path, "/api/"), "/"), "/", ".")
	if event == "" {
		e.diagnostics.Log(diagnostics.Warn, "app", "command.unknown", "Неизвестное действие отклонено")
		return
	}
	level := diagnostics.Info
	switch path {
	case "/api/controls/status", "/api/controls", "/api/exploration/query",
		"/api/ship-fits", "/api/ship-fits/get", "/api/ship-fits/materials", "/api/ship-fits/fleet", "/api/ship-builds",
		"/api/engineering/itinerary", "/api/engineering/current-ship", "/api/notifications", "/api/second-screen",
		"/api/theme/preview", "/api/desktop", "/api/ship-route":
		if err == nil {
			return
		}
		level = diagnostics.Debug
	}
	group, _, _ := strings.Cut(event, ".")
	switch action {
	case "add", "update", "remove", "delete", "clear", "complete", "reopen", "select",
		"new-project", "select-project", "update-project", "remove-project", "record-delivery", "remove-delivery",
		"add-building", "update-building", "remove-building", "opportunities", "save", "cancel",
		"attach", "start", "stop", "rotate", "revoke", "backup", "restore", "activate", "import", "export":
		event += "." + action
	}
	e.recordOperation(diagnosticSubsystem(group), event, started, level, result, err)
}

func diagnosticSubsystem(group string) string {
	switch group {
	case "bookmarks", "objectives", "session":
		return "profile"
	case "journal-dir", "rescan":
		return "journal"
	case "desktop", "navigation", "theme":
		return "settings"
	case "integrations":
		return "inara"
	case "colonization":
		return "colony"
	case "carrierRoute", "carrier-trade":
		return "carrier"
	case "trading", "tradeRoutes", "market":
		return "trade"
	case "politics":
		return "powerplay"
	case "riches", "ship-route", "shipRouteSearch":
		return "exploration"
	case "surface-navigation":
		return "surface"
	case "ship-builds", "ship-fits":
		return "fits"
	case "second-screen", "notifications", "learning":
		return "ui"
	default:
		return group
	}
}

// RecordOperation records a native operation. Subsystem and event come from
// code constants, never request parameters. Result contents are not persisted.
func (e *Engine) RecordOperation(subsystem, event string, started time.Time, result M, err error) {
	e.recordOperation(subsystem, event, started, diagnostics.Info, result, err)
}

func (e *Engine) recordOperation(subsystem, event string, started time.Time, level diagnostics.Level, result M, err error) {
	message := "Действие выполнено"
	switch {
	case errors.Is(err, context.Canceled) || result["status"] == "cancelled" || result["cancelled"] == true:
		level, message = diagnostics.Debug, "Действие отменено"
	case errors.Is(err, context.DeadlineExceeded):
		level, message = diagnostics.Warn, "Истекло время ожидания"
	case errors.Is(err, os.ErrPermission):
		level, message = diagnostics.Error, "Отказано в доступе"
	case errors.Is(err, os.ErrNotExist):
		level, message = diagnostics.Error, "Файл не найден"
	case err != nil || result["status"] == "error" || v.Str(result["error"]) != "":
		level, message = diagnostics.Error, "Ошибка при выполнении действия"
	case result["status"] == "unavailable":
		level, message = diagnostics.Warn, "Данные недоступны"
	case result["status"] == "invalid":
		level, message = diagnostics.Warn, "Некорректные параметры"
	case result["status"] == "disabled":
		level, message = diagnostics.Debug, "Действие отключено"
	case result["partial"] == true || len(v.Obj(result["errors"])) > 0:
		level, message = diagnostics.Warn, "Действие выполнено частично"
	case result["started"] == true:
		message = "Действие запущено"
	case result["cached"] == true:
		message = "Данные загружены из кэша"
	case result["ok"] == false:
		level, message = diagnostics.Warn, "Действие не выполнено"
	}
	if !started.IsZero() {
		message = fmt.Sprintf("%s%s%d ms", message, diagnosticSeparator, time.Since(started).Milliseconds())
	}
	e.diagnostics.Log(level, subsystem, event, message)
}

func (e *Engine) sendInaraBatch(ctx context.Context, options inara.SendOptions) (inara.BatchResult, error) {
	started := time.Now()
	result, err := inara.SendBatch(ctx, options)
	accepted := 0
	for _, event := range result.Results {
		if event.OK {
			accepted++
		}
	}
	e.RecordOperation("inara", "batch.send", started, M{"partial": accepted < len(options.Events)}, err)
	return result, err
}

func (e *Engine) observedHTTP(client, fallback *http.Client) *http.Client {
	if client == nil {
		client = fallback
	}
	if client == nil {
		client = &http.Client{}
	}
	copy := *client
	if copy.Transport == nil {
		copy.Transport = http.DefaultTransport
	}
	copy.Transport = diagnosticTransport{base: copy.Transport, store: e.diagnostics}
	return &copy
}

type diagnosticTransport struct {
	base  http.RoundTripper
	store *diagnostics.Store
}

func (t diagnosticTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	started := time.Now()
	response, err := t.base.RoundTrip(request)
	if errors.Is(err, context.Canceled) {
		return response, err
	}
	elapsed := time.Since(started)
	level, message := diagnostics.Debug, "Сетевой запрос завершён"
	if err != nil {
		level, message = diagnostics.Error, "Сетевой запрос завершился ошибкой"
		var nerr net.Error
		if errors.As(err, &nerr) && nerr.Timeout() {
			level, message = diagnostics.Warn, "Истекло время ожидания"
		}
	} else if response != nil {
		switch {
		case response.StatusCode >= 500:
			level = diagnostics.Error
		case response.StatusCode >= 400:
			level = diagnostics.Warn
		case elapsed < time.Second:
			return response, nil
		}
		message = fmt.Sprintf("%s%sHTTP %d", message, diagnosticSeparator, response.StatusCode)
	}
	t.store.Log(level, diagnosticProvider(request.URL.Hostname()), "http.request", fmt.Sprintf("%s%s%d ms", message, diagnosticSeparator, elapsed.Milliseconds()))
	return response, err
}

func diagnosticProvider(host string) string {
	host = strings.ToLower(host)
	for _, provider := range []struct{ domain, label string }{
		{"frontier.co.uk", "frontier"}, {"elitedangerous.com", "frontier"}, {"inara.cz", "inara"},
		{"edsm.net", "edsm"}, {"spansh.co.uk", "spansh"}, {"github.com", "github"},
		{"githubusercontent.com", "github"}, {"edtools.cc", "edtools"},
	} {
		if host == provider.domain || strings.HasSuffix(host, "."+provider.domain) {
			return provider.label
		}
	}
	return "network"
}
