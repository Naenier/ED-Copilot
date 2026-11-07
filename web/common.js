(function () {
  "use strict";

  const numberFormat = new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 1 });
  const integerFormat = new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 0 });
  const methods = {
    all: "Все способы",
    laser: "Лазерная",
    core: "Раскол ядра",
    subsurface: "Подповерхностная",
    surface: "Поверхностные залежи астероидов",
    "surface-planet": "Планетарная добыча",
    raw: "Сбор сырья",
  };
  const ringTypes = {
    metallic: "Металлическое",
    metalrich: "Богатое металлами",
    rocky: "Каменистое",
    icy: "Ледяное",
    eRingClass_Icy: "Ледяное",
    eRingClass_Rocky: "Каменистое",
    eRingClass_MetalRich: "Богатое металлами",
    eRingClass_Metalic: "Металлическое",
    Icy: "Ледяное",
    Rocky: "Каменистое",
    "Metal Rich": "Богатое металлами",
    Metallic: "Металлическое",
  };
  const reserves = {
    PristineResources: "Нетронутые",
    MajorResources: "Крупные",
    CommonResources: "Обычные",
    LowResources: "Малые",
    DepletedResources: "Истощённые",
    Pristine: "Нетронутые",
    Major: "Крупные",
    Common: "Обычные",
    Low: "Малые",
    Depleted: "Истощённые",
  };
  const tierLabels = { low: "Обычная", medium: "Средняя", high: "Высокая", unknown: "Оценка нужна" };
  const tierValues = { low: 1, medium: 2, high: 3, unknown: 0 };

  function escapeHtml(value) {
    return String(value ?? "").replace(
      /[&<>"']/g,
      (character) =>
        ({
          "&": "&amp;",
          "<": "&lt;",
          ">": "&gt;",
          '"': "&quot;",
          "'": "&#39;",
        })[character],
    );
  }

  function number(value, integer = false) {
    if (value == null || !Number.isFinite(Number(value))) return "—";
    return (integer ? integerFormat : numberFormat).format(Number(value));
  }

  function credits(value) {
    if (value == null || !Number.isFinite(Number(value))) return "—";
    const amount = Number(value);
    if (Math.abs(amount) >= 1e9) return number(amount / 1e9) + " млрд кр.";
    if (Math.abs(amount) >= 1e6) return number(amount / 1e6) + " млн кр.";
    if (Math.abs(amount) >= 1e4) return number(amount / 1e3) + " тыс. кр.";
    return number(amount, true) + " кр.";
  }

  function dateTime(value, dateOnly = false) {
    if (!value) return "—";
    const date = new Date(value);
    if (!Number.isFinite(date.getTime())) return "—";
    return date.toLocaleString(
      "ru-RU",
      dateOnly
        ? { day: "2-digit", month: "short", year: "numeric" }
        : { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" },
    );
  }

  function duration(seconds) {
    if (seconds == null || seconds === "" || !Number.isFinite(Number(seconds))) return "—";
    const minutes = Math.max(0, Math.floor(Number(seconds) / 60));
    return minutes < 60 ? minutes + " мин" : Math.floor(minutes / 60) + " ч " + (minutes % 60) + " мин";
  }

  function pill(text, style) {
    return (
      '<span class="pill ' + (style ? "pill-" + escapeHtml(style) : "") + '">' + escapeHtml(text) + "</span>"
    );
  }

  function confidence(target) {
    if (target.miningSites > 0 && target.compositionUnknown)
      return pill("Места добычи подтверждены", "confirmed");
    if (target.confidence === "confirmed") return pill("Подтверждённые данные", "confirmed");
    if (target.confidence === "potential") return pill("Кандидат для разведки", "potential");
    return pill("Нужна проверка", "muted");
  }

  function tier(target) {
    const level =
      typeof target === "string" ? target : target.compositionUnknown ? "unknown" : target.tier || "unknown";
    return pill(tierLabels[level] || level, level === "unknown" ? "potential" : level);
  }

  async function api(path, data) {
    const response = await fetch(
      path,
      data === undefined
        ? {}
        : {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(data),
          },
    );
    const result = await response.json();
    if (!response.ok || result.ok === false) throw new Error(result.error || "Запрос не выполнен");
    return result;
  }

  function subscribe(onState, onConnection) {
    let closed = false;
    let source;
    let retryTimer;
    async function refresh() {
      try {
        const result = await api("/api/state");
        if (!closed) onState(result.state || result);
      } catch (error) {
        if (!closed) onConnection(false, error.message);
      }
    }
    function receive(event) {
      try {
        const result = JSON.parse(event.data);
        onState(result.state || result);
        onConnection(true);
      } catch {
        onConnection(false, "Не удалось прочитать обновление состояния");
      }
    }
    refresh();
    if (typeof EventSource === "function") {
      source = new EventSource("/api/events");
      source.addEventListener("state", receive);
      source.onmessage = receive;
      source.onopen = () => onConnection(true);
      source.onerror = () => onConnection(false, "Восстановление соединения с журналом");
    } else {
      retryTimer = setInterval(refresh, 1500);
    }
    return {
      refresh,
      close() {
        closed = true;
        source?.close();
        clearInterval(retryTimer);
      },
    };
  }

  async function copyText(text) {
    if (!text) return;
    if (window.edMining?.copyText) await window.edMining.copyText(text);
    else await navigator.clipboard.writeText(text);
  }

  function resourceValue(cargo, resources) {
    const prices = new Map((resources || []).map((resource) => [resource.key, resource.price]));
    let total = 0;
    let priced = 0;
    let unknownTons = 0;
    for (const item of cargo || []) {
      if (/^(?:drones|limpet|limpets)$/i.test(item.key)) continue;
      const price = prices.get(item.key);
      if (price == null || !Number.isFinite(Number(price)) || Number(price) <= 0)
        unknownTons += item.count || 0;
      else {
        total += Number(price) * (item.count || 0);
        priced += item.count || 0;
      }
    }
    return { total: priced > 0 ? total : null, priced, unknownTons };
  }

  function eligibleTarget(target, state, overrides = {}) {
    const preferences = { ...state.settings, ...overrides };
    const unknownSite =
      target.miningSites > 0 && target.compositionUnknown && target.confidence === "confirmed";
    if (!unknownSite && (tierValues[target.tier] || 0) < (tierValues[preferences.minTier] || 0)) return false;
    const method = preferences.method || "all";
    const available = target.methods || [];
    if (method !== "all" && !available.includes(method)) return false;
    const laserOnly = available.length > 0 && available.every((entry) => entry === "laser");
    if (
      preferences.hideDepleted &&
      /depleted|истощ/i.test(target.reserveKey || target.reserve || "") &&
      (method === "laser" || laserOnly)
    )
      return false;
    return !(
      target.distanceLs != null &&
      preferences.maxArrivalLs > 0 &&
      target.distanceLs > preferences.maxArrivalLs
    );
  }

  function resourceName(resources, value) {
    const normalize = (text) =>
      String(text ?? "")
        .replace(/[^a-zа-яё0-9]/gi, "")
        .toLowerCase();
    const key = normalize(value);
    const resource = (resources || []).find((item) =>
      [item.key, item.name, item.english, ...(item.aliases || [])].some((name) => normalize(name) === key),
    );
    return resource?.name || value || "";
  }

  function safeUrl(value) {
    try {
      const url = new URL(value);
      return ["https:", "http:"].includes(url.protocol) ? url.href : "";
    } catch {
      return "";
    }
  }

  window.MiningUI = {
    h: escapeHtml,
    n: number,
    credits,
    dateTime,
    duration,
    pill,
    confidence,
    tier,
    tierValues,
    methods,
    ringTypes,
    reserves,
    api,
    subscribe,
    copyText,
    resourceValue,
    eligibleTarget,
    resourceName,
    safeUrl,
  };
})();
