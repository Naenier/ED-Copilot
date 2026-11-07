"use strict";

const { keyOf, distance } = require("./targets.cjs");
const catalog = require("./catalog.cjs");
const { spanshSystemFromDump } = require("./spansh.cjs");
const { setTimeout: delay } = require("node:timers/promises");

const finite = (value) => value != null && Number.isFinite(Number(value)) ? Number(value) : null;
const coordinates = (value) => value && [value.x, value.y, value.z].every((number) => finite(number) != null)
  ? [Number(value.x), Number(value.y), Number(value.z)] : null;

async function fetchJson(url, options = {}, signal) {
  const timeout = AbortSignal.timeout(20000);
  const response = await fetch(url, { ...options, signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
    headers: { "User-Agent": "ED-Copilot/1.0.0 (Elite Dangerous local companion)", Accept: "application/json", ...options.headers } });
  if (!response.ok) throw new Error("Сервис ответил HTTP " + response.status);
  const length = Number(response.headers.get("content-length") || 0);
  if (length > 12000000) throw new Error("Ответ сервиса слишком большой.");
  return response.json();
}

function edsmSystem(payload, reference) {
  const info = payload?.bodies ? payload : { ...reference, bodies: [] };
  return {
    address: info.id64 ?? reference.id64 ?? null,
    name: info.name ?? reference.name,
    coordinates: coordinates(reference.coords) ?? coordinates(info.coords),
    bodies: (info.bodies ?? []).map((body) => ({
      id: body.bodyId ?? body.id, name: body.name, source: "edsm", timestamp: body.updateTime ?? new Date().toISOString(),
      landable: body.isLandable === true, distanceLs: finite(body.distanceToArrival),
      gravity: finite(body.gravity), reserve: body.reserveLevel ?? "",
      materials: Object.entries(body.materials ?? {}).map(([key, percent]) => ({ key: keyOf(key), percent: Number(percent) })),
      rings: (body.rings ?? []).map((ring) => ({ name: ring.name, type: ring.type ?? "", signals: [] })),
    })),
  };
}

function spanshSystems(payload) {
  const systems = new Map();
  for (const body of payload.results ?? []) {
    const name = body.system_name;
    if (!name || !body.name) continue;
    let system = systems.get(name);
    if (!system) {
      system = { name, address: body.system_id64 ?? null, coordinates: coordinates({ x: body.system_x, y: body.system_y, z: body.system_z }), bodies: [] };
      systems.set(name, system);
    }
    system.bodies.push({
      id: body.body_id ?? body.id64 ?? body.name, name: body.name, source: "spansh", timestamp: body.updated_at ?? new Date().toISOString(),
      landable: body.is_landable === true, distanceLs: finite(body.distance_to_arrival),
      gravity: finite(body.gravity), reserve: body.reserve_level ?? "",
      materials: (Array.isArray(body.materials) ? body.materials : []).map((entry) => ({
        key: keyOf(entry.name), percent: finite(entry.share ?? entry.percent) ?? 0,
      })),
      rings: (body.rings ?? []).map((ring) => ({
        name: ring.name, type: ring.type ?? "",
        signals: (ring.signals ?? []).map((signal) => ({ key: keyOf(signal.name), name: signal.name, count: signal.count })),
      })),
    });
  }
  return [...systems.values()];
}

class MiningSearch {
  constructor(engine) {
    this.engine = engine;
    this.controller = null;
  }

  cancel() {
    this.controller?.abort();
  }

  async start({ system, radiusLy, resourceKey }) {
    if (this.controller) throw new Error("Поиск уже выполняется. Дождитесь завершения или отмените его.");
    const center = String(system || this.engine.commander.system).trim();
    if (!center || center.length > 150) throw new Error("Укажите название системы.");
    const radius = Math.max(5, Math.min(100, Number(radiusLy) || this.engine.preferences.radiusLy));
    this.controller = new AbortController();
    const signal = this.controller.signal;
    this.engine.searchState = { running: true, system: center, radiusLy: radius, checked: 0, total: 0,
      status: "Определение координат системы…", error: null, finishedAt: null };
    this.engine.emit();
    void this.run(center, radius, resourceKey, signal).finally(() => {
      this.engine.searchState.running = false;
      this.engine.searchState.finishedAt = new Date().toISOString();
      this.controller = null;
      this.engine.emit();
    });
  }

  async run(center, radius, resourceKey, signal) {
    const engine = this.engine;
    const errors = [];
    try {
      let reference = [...engine.world.values()].find((entry) => entry.name.toLowerCase() === center.toLowerCase() && entry.coordinates);
      if (reference) reference = { name: reference.name, id64: reference.address,
        coords: { x: reference.coordinates[0], y: reference.coordinates[1], z: reference.coordinates[2] } };
      else reference = await fetchJson("https://www.edsm.net/api-v1/system?" + new URLSearchParams({
        systemName: center, showCoordinates: "1", showId: "1",
      }), {}, signal);
      const from = coordinates(reference.coords);
      if (!from) throw new Error("Координаты системы не найдены. Посетите её или проверьте название.");
      let referenceDumpLoaded = false;
      let dumpFailures = 0;
      if (reference.id64 != null) {
        engine.searchState.status = "Проверка планетарных участков Spansh в системе " + center + "…";
        engine.emit();
        try {
          const payload = await fetchJson("https://spansh.co.uk/api/dump/" + encodeURIComponent(String(reference.id64)), {}, signal);
          engine.mergeRemoteSystem(spanshSystemFromDump(payload));
          referenceDumpLoaded = true;
        } catch (error) {
          if (signal.aborted) throw error;
          errors.push("Spansh, планетарные участки: " + error.message);
          dumpFailures += 1;
        }
      }
      const selected = catalog.getResource?.(resourceKey);
      const names = selected?.kind === "commodity" && selected.ringTypes.length ? [selected.english]
        : ["Platinum", "Painite", "Monazite", "Musgravite", "Low Temperature Diamonds"];
      engine.searchState.total = names.length;
      for (const name of names) {
        signal.throwIfAborted();
        engine.searchState.status = "Поиск подтверждённых горячих точек: " + (catalog.getResource?.(name)?.name ?? name);
        engine.emit();
        try {
          const payload = await fetchJson("https://spansh.co.uk/api/bodies/search", {
            method: "POST", headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              filters: { ring_signals: [{ name, value: [1, 9999], comparison: "<=>" }] },
              sort: [{ distance: { direction: "asc" } }], size: 50, page: 0,
              reference_coords: { x: from[0], y: from[1], z: from[2] },
            }),
          }, signal);
          for (const entry of spanshSystems(payload)) {
            const ly = distance(from, entry.coordinates);
            if (ly != null && ly <= radius) engine.mergeRemoteSystem(entry);
          }
        } catch (error) {
          if (signal.aborted) throw error;
          errors.push("Spansh: " + error.message);
        }
        engine.searchState.checked += 1;
        engine.emit();
        await delay(350, undefined, { signal });
      }

      engine.searchState.status = "Поиск ближайших систем для проверки колец и планетарных участков…";
      engine.emit();
      let nearby = [];
      try {
        const response = await fetchJson("https://www.edsm.net/api-v1/sphere-systems?" + new URLSearchParams({
          systemName: center, radius: String(radius), showCoordinates: "1", showId: "1",
        }), {}, signal);
        nearby = Array.isArray(response) ? response : [];
      } catch (error) {
        if (signal.aborted) throw error;
        errors.push("EDSM: " + error.message);
      }
      // Requesting every known system in a populated sphere would overload the community API.
      const systems = [reference, ...nearby.filter((entry) => entry.name?.toLowerCase() !== center.toLowerCase())]
        .sort((a, b) => (Number(a.distance) || 0) - (Number(b.distance) || 0)).slice(0, 20);
      engine.searchState.total += systems.length;
      for (const entry of systems) {
        signal.throwIfAborted();
        engine.searchState.status = "Проверка тел: " + entry.name;
        engine.emit();
        try {
          let complete = referenceDumpLoaded && entry.name.toLowerCase() === center.toLowerCase();
          if (!complete && entry.id64 != null && dumpFailures < 2) {
            try {
              const payload = await fetchJson("https://spansh.co.uk/api/dump/" + encodeURIComponent(String(entry.id64)), {}, signal);
              engine.mergeRemoteSystem(spanshSystemFromDump(payload));
              complete = true;
              dumpFailures = 0;
            } catch (error) {
              if (signal.aborted) throw error;
              dumpFailures += 1;
              errors.push(entry.name + ", Spansh: " + error.message);
            }
          }
          if (!complete) {
            const response = await fetchJson("https://www.edsm.net/api-system-v1/bodies?" + new URLSearchParams({
              systemName: entry.name,
            }), {}, signal);
            engine.mergeRemoteSystem(edsmSystem(response, entry));
          }
        } catch (error) {
          if (signal.aborted) throw error;
          errors.push(entry.name + ": " + error.message);
        }
        engine.searchState.checked += 1;
        engine.emit();
        await delay(250, undefined, { signal });
      }
      engine.searchState.status = "Поиск завершён. Добавлены горячие точки и известные тела до 20 ближайших систем. Планетарные сигналы взяты из доступных данных Spansh.";
      if (errors.length) {
        engine.searchState.error = [...new Set(errors)].slice(0, 3).join(" · ");
        engine.notice("Часть источников поиска недоступна. Локальные цели и полученные результаты доступны.", "warn");
      }
    } catch (error) {
      engine.searchState.status = signal.aborted ? "Поиск отменён. Полученные результаты сохранены." : "Поиск не завершён.";
      engine.searchState.error = signal.aborted ? null : error.message;
    }
  }
}

module.exports = { MiningSearch, edsmSystem, spanshSystems };
